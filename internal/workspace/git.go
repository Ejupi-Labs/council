package workspace

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (m *Manager) git(ctx context.Context, dir string, input []byte, args ...string) ([]byte, error) {
	argv := []string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + m.hooks, "-c", "diff.external=", "-c", "core.attributesFile=/dev/null", "-c", "core.quotePath=false"}
	argv = append(argv, args...)
	c := exec.CommandContext(ctx, "git", argv...)
	c.Dir = dir
	c.Stdin = bytes.NewReader(input)
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if !strings.HasPrefix(strings.ToUpper(k), "GIT_") {
			c.Env = append(c.Env, v)
		}
	}
	c.Env = append(c.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	var out, bad cappedBuffer
	c.Stdout = &out
	c.Stderr = &bad
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("%w Git operation %s could not complete.", ErrBlocked, args[0])
	}
	if out.overflow || bad.overflow {
		return nil, fmt.Errorf("%w Git metadata exceeds its capture limit.", ErrBlocked)
	}
	return out.Bytes(), nil
}

type cappedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	const bound = 128 << 20
	if b.Len()+len(p) > bound {
		b.overflow = true
		p = p[:max(0, bound-b.Len())]
	}
	b.Buffer.Write(p)
	return n, nil
}

func safePath(v string) bool {
	if v == "" || !utf8.ValidString(v) || strings.ContainsAny(v, "\x00\\") || filepath.IsAbs(v) || strings.HasPrefix(v, "/") {
		return false
	}
	for _, p := range strings.Split(v, "/") {
		if p == "" || p == "." || p == ".." || strings.EqualFold(p, ".git") {
			return false
		}
	}
	return true
}

func parseIndexEntries(raw []byte) ([]IndexEntry, error) {
	var out []IndexEntry
	for _, record := range bytes.Split(raw, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		meta, path, ok := bytes.Cut(record, []byte{'\t'})
		fields := strings.Fields(string(meta))
		if !ok || len(fields) != 3 || !safePath(string(path)) {
			return nil, fmt.Errorf("%w Unsupported index path or format.", ErrBlocked)
		}
		if fields[2] != "0" {
			return nil, fmt.Errorf("%w Unmerged indexes cannot be captured.", ErrBlocked)
		}
		if fields[0] == "160000" {
			return nil, fmt.Errorf("%w Submodules cannot be captured.", ErrBlocked)
		}
		if fields[0] != "100644" && fields[0] != "100755" && fields[0] != "120000" {
			return nil, fmt.Errorf("%w Sparse or special index entries cannot be captured.", ErrBlocked)
		}
		out = append(out, IndexEntry{Path: string(path), Mode: fields[0], Object: fields[1]})
		if len(out) > MaxEntries {
			return nil, fmt.Errorf("%w Source has more than 100000 entries.", ErrBlocked)
		}
	}
	return out, nil
}

// Validate flags from the original raw index without refreshing or modifying it.
// Versions 2/3 use padded names; v4 uses bounded previous-name prefix compression.
// A zero checksum is Git's documented index.skipHash encoding; source fidelity is
// independently captured by the retained raw index and manifest SHA-256.
func validateRawIndex(b []byte) error {
	if len(b) < 32 || string(b[:4]) != "DIRC" {
		return fmt.Errorf("%w Invalid Git index.", ErrBlocked)
	}
	v := binary.BigEndian.Uint32(b[4:8])
	if v != 2 && v != 3 && v != 4 {
		return fmt.Errorf("%w Git index version %d is unsupported.", ErrBlocked, v)
	}
	checksum := sha1.Sum(b[:len(b)-20])
	if !bytes.Equal(b[len(b)-20:], make([]byte, 20)) && !bytes.Equal(checksum[:], b[len(b)-20:]) {
		return fmt.Errorf("%w The Git index checksum is invalid.", ErrBlocked)
	}
	count := binary.BigEndian.Uint32(b[8:12])
	if count > MaxEntries {
		return fmt.Errorf("%w Source has more than 100000 index entries.", ErrBlocked)
	}
	o := 12
	previousLength := 0
	for range count {
		start := o
		if o+62 > len(b)-20 {
			return fmt.Errorf("%w Truncated Git index.", ErrBlocked)
		}
		flags := binary.BigEndian.Uint16(b[o+60 : o+62])
		o += 62
		if flags&0x4000 != 0 {
			if o+2 > len(b)-20 {
				return ErrBlocked
			}
			extended := binary.BigEndian.Uint16(b[o : o+2])
			o += 2
			if extended&0x2000 != 0 {
				return fmt.Errorf("%w Intent-to-add indexes cannot be captured.", ErrBlocked)
			}
			if extended&0x4000 != 0 {
				return fmt.Errorf("%w Sparse indexes cannot be captured.", ErrBlocked)
			}
		}
		removed := 0
		if v == 4 {
			if o >= len(b)-20 {
				return ErrBlocked
			}
			c := b[o]
			o++
			removed = int(c & 0x7f)
			for c&0x80 != 0 {
				if removed > previousLength || o >= len(b)-20 {
					return ErrBlocked
				}
				removed = (removed + 1) << 7
				c = b[o]
				o++
				removed += int(c & 0x7f)
			}
			if removed > previousLength {
				return ErrBlocked
			}
		}
		if o > len(b)-20 {
			return ErrBlocked
		}
		end := bytes.IndexByte(b[o:len(b)-20], 0)
		if end < 0 {
			return ErrBlocked
		}
		nameLength := end
		if v == 4 {
			nameLength += previousLength - removed
		}
		if (nameLength == 0 && count > 0 && v == 4) || (flags&0xfff != 0xfff && int(flags&0xfff) != nameLength) {
			return ErrBlocked
		}
		previousLength = nameLength
		o += end + 1
		if v != 4 {
			for (o-start)%8 != 0 {
				o++
			}
		}
		if o > len(b)-20 {
			return ErrBlocked
		}
	}
	for o < len(b)-20 {
		if o+8 > len(b)-20 {
			return ErrBlocked
		}
		name := string(b[o : o+4])
		size := int(binary.BigEndian.Uint32(b[o+4 : o+8]))
		o += 8
		if size > len(b)-20-o {
			return ErrBlocked
		}
		if name == "link" {
			return fmt.Errorf("%w Split indexes cannot be captured.", ErrBlocked)
		}
		if name == "sdir" {
			return fmt.Errorf("%w Sparse indexes cannot be captured.", ErrBlocked)
		}
		o += size
	}
	return nil
}

func readIndex(path string) ([]byte, error) {
	i, err := os.Lstat(path)
	if err != nil || !i.Mode().IsRegular() || i.Size() > 64<<20 {
		return nil, fmt.Errorf("%w Git index is unavailable or unsupported.", ErrBlocked)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrBlocked
	}
	defer f.Close()
	j, err := f.Stat()
	if err != nil || !os.SameFile(i, j) {
		return nil, ErrBlocked
	}
	b, err := ioReadBounded(f, 64<<20)
	if err != nil {
		return nil, err
	}
	k, err := f.Stat()
	if err != nil || !os.SameFile(i, k) || i.Size() != k.Size() || !i.ModTime().Equal(k.ModTime()) {
		return nil, fmt.Errorf("%w Git index changed during capture.", ErrBlocked)
	}
	return b, validateRawIndex(b)
}

func (m *Manager) index(ctx context.Context, dir string) ([]IndexEntry, error) {
	b, e := m.git(ctx, dir, nil, "ls-files", "--stage", "--sparse", "-z")
	if e != nil {
		return nil, e
	}
	return parseIndexEntries(b)
}
func (m *Manager) value(ctx context.Context, dir string, args ...string) (string, error) {
	b, e := m.git(ctx, dir, nil, args...)
	return strings.TrimSpace(string(b)), e
}

func excluded(path string) string {
	p := strings.ToLower(path)
	parts := strings.Split(p, "/")
	for _, part := range parts {
		switch part {
		case ".council":
			return "Council private collaboration state"
		case ".codex", ".claude", ".gemini", ".opencode":
			return "Vendor authentication or private harness state"
		}
	}
	if p == ".claude.json" || p == ".claude.json.backup" {
		return "Vendor authentication or private harness state"
	}
	return ""
}

func within(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func indexEqual(a, b []IndexEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func modeNumber(mode string) uint32 { v, _ := strconv.ParseUint(mode, 8, 32); return uint32(v) }

var _ = errors.Is
