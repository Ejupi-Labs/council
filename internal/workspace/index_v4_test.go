package workspace

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestActualGitV4PrefixCompressionAndOptionalHash(t *testing.T) {
	for _, skipHash := range []string{"false", "true"} {
		t.Run(skipHash, func(t *testing.T) {
			repo := t.TempDir()
			git := func(args ...string) {
				t.Helper()
				c := exec.Command("git", args...)
				c.Dir = repo
				if b, err := c.CombinedOutput(); err != nil {
					t.Fatalf("%v %s", err, b)
				}
			}
			git("init", "--quiet")
			git("config", "index.version", "4")
			git("config", "index.skipHash", skipHash)
			long := strings.Repeat("a", 150)
			if err := os.Mkdir(filepath.Join(repo, long), 0700); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{long + "/a.txt", long + "/b.txt", "z.txt"} {
				if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(name)), []byte(name), 0600); err != nil {
					t.Fatal(err)
				}
			}
			git("add", "--all")
			raw, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
			if err != nil {
				t.Fatal(err)
			}
			if binary.BigEndian.Uint32(raw[4:8]) != 4 {
				t.Fatal("Fixture did not produce index v4")
			}
			if err = validateRawIndex(raw); err != nil {
				t.Fatal(err)
			}
			if skipHash == "false" {
				corrupt := bytes.Clone(raw)
				corrupt[15] ^= 1
				if validateRawIndex(corrupt) == nil {
					t.Fatal("Accepted corrupt checksum")
				}
			}
		})
	}
}

func TestV4InvalidPrefixCannotReadPastIndex(t *testing.T) {
	raw := make([]byte, 12+62+3+20)
	copy(raw, "DIRC")
	binary.BigEndian.PutUint32(raw[4:8], 4)
	binary.BigEndian.PutUint32(raw[8:12], 1)
	raw[12+62] = 0xff
	raw[12+63] = 0x7f
	h := sha1.Sum(raw[:len(raw)-20])
	copy(raw[len(raw)-20:], h[:])
	if validateRawIndex(raw) == nil {
		t.Fatal("Accepted a prefix removal beyond the previous path")
	}
}
