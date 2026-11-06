package evidence

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type Store struct {
	lock          *localstate.Lock
	root          *os.Root
	sink          Sink
	outputMu      sync.Mutex
	outputs       map[string]outputReference
	outputsClosed bool
	proofMu       sync.Mutex
	proofs        map[string]rangeProof
}

func New(lock *localstate.Lock, sink Sink) (*Store, error) {
	if lock == nil || sink == nil {
		return nil, errors.New("Evidence needs private ownership and metadata storage.")
	}
	if err := lock.CheckHeld(); err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(lock.Root())
	if err != nil {
		return nil, err
	}
	if err = r.Mkdir("artifacts", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		r.Close()
		return nil, err
	}
	d, err := r.Open("artifacts")
	if err != nil {
		r.Close()
		return nil, err
	}
	err = localstate.ProtectPrivateFile(d, true)
	d.Close()
	if err != nil {
		r.Close()
		return nil, err
	}
	return &Store{lock: lock, root: r, sink: sink, outputs: map[string]outputReference{}}, nil
}

func (s *Store) Close() error { return errors.Join(s.closeOutputs(), s.root.Close()) }

func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func validID(id string) bool {
	b, e := hex.DecodeString(id)
	return e == nil && len(b) == 16 && strings.ToLower(id) == id
}

func (s *Store) Create(ctx context.Context, attemptID, kind string, maxBytes int64) (*Stream, error) {
	return s.create(ctx, attemptID, kind, maxBytes, true)
}

func (s *Store) create(ctx context.Context, attemptID, kind string, maxBytes int64, publish bool) (*Stream, error) {
	if !validID(attemptID) || kind == "" || len(kind) > 64 || maxBytes < 0 {
		return nil, errors.New("Invalid artifact metadata or byte limit.")
	}
	if err := s.lock.CheckHeld(); err != nil {
		return nil, err
	}
	id, err := NewID()
	if err != nil {
		return nil, err
	}
	rel := "artifacts/" + id + ".bin"
	f, err := s.root.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*Stream, error) { f.Close(); return nil, e }
	if err = localstate.ProtectPrivateFile(f, false); err != nil {
		return fail(err)
	}
	identity, err := fileIdentity(f)
	if err != nil {
		return fail(err)
	}
	if err = f.Sync(); err != nil {
		return fail(err)
	}
	if err = localstate.SyncPrivateDirectory(filepath.Join(s.lock.Root(), "artifacts")); err != nil {
		return fail(err)
	}
	m := Metadata{ID: id, AttemptID: attemptID, Kind: kind, ByteLength: "0", Completeness: "collecting", CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), RelativePath: rel, Identity: identity}
	if publish {
		if err = s.sink.SaveArtifact(ctx, m); err != nil {
			return fail(err)
		}
	}
	return &Stream{store: s, file: f, metadata: m, maxBytes: maxBytes, deferMetadata: !publish}, nil
}

func (s *Store) Put(ctx context.Context, attemptID, kind string, data []byte) (Metadata, error) {
	w, err := s.create(ctx, attemptID, kind, int64(len(data)), false)
	if err != nil {
		return Metadata{}, err
	}
	if _, err = w.Write(data); err != nil {
		m, closeErr := w.Close(ctx, false)
		return m, errors.Join(err, closeErr)
	}
	return w.Close(ctx, true)
}

func decimal(v string) (int64, error) {
	if v == "" {
		v = "0"
	}
	if len(v) > 19 || len(v) > 1 && v[0] == '0' {
		return 0, errors.New("Invalid exact byte offset.")
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return 0, errors.New("Invalid exact byte offset.")
		}
	}
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n < 0 {
		return 0, errors.New("Invalid exact byte offset.")
	}
	return n, nil
}

func (s *Store) open(ctx context.Context, id string, fresh bool) (Metadata, *os.File, int64, error) {
	if !validID(id) {
		return Metadata{}, nil, 0, errors.New("Invalid artifact identity.")
	}
	if err := s.lock.CheckHeld(); err != nil {
		return Metadata{}, nil, 0, err
	}
	m, err := s.sink.LoadArtifact(ctx, id)
	if err != nil {
		return m, nil, 0, err
	}
	if m.ID != id || m.RelativePath != "artifacts/"+id+".bin" || m.Identity == "" {
		return m, nil, 0, ErrCorrupt
	}
	n, err := decimal(m.ByteLength)
	if err != nil {
		return m, nil, 0, ErrCorrupt
	}
	i, err := s.root.Lstat(m.RelativePath)
	if err != nil || !i.Mode().IsRegular() {
		return m, nil, 0, ErrCorrupt
	}
	f, err := s.root.Open(m.RelativePath)
	if err != nil {
		return m, nil, 0, ErrCorrupt
	}
	bad := func() (Metadata, *os.File, int64, error) { f.Close(); return m, nil, 0, ErrCorrupt }
	if err = localstate.VerifyPrivateFile(f, false); err != nil {
		return bad()
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(i, opened) || !opened.Mode().IsRegular() || opened.Size() < n || m.Closed && opened.Size() != n {
		return bad()
	}
	identity, err := fileIdentity(f)
	if err != nil || identity != m.Identity {
		return bad()
	}
	if fresh && m.Closed && m.Completeness == "complete" {
		if m.SHA256 == nil {
			return bad()
		}
		h := sha256.New()
		if _, err = io.Copy(h, contextReader{ctx, f}); err != nil {
			if ctx.Err() != nil {
				f.Close()
				return m, nil, 0, ctx.Err()
			}
			return bad()
		}
		if hex.EncodeToString(h.Sum(nil)) != *m.SHA256 {
			return bad()
		}
		if _, err = f.Seek(0, 0); err != nil {
			return bad()
		}
	}
	return m, f, n, nil
}

func (s *Store) VerifyClosed(ctx context.Context, id string) (Metadata, error) {
	m, f, _, err := s.open(ctx, id, true)
	if err != nil {
		return m, err
	}
	defer f.Close()
	if !m.Closed || m.Completeness != "complete" || m.SHA256 == nil {
		return m, errors.New("Evidence is not complete and immutable.")
	}
	return m, nil
}

func (s *Store) Read(ctx context.Context, id string, opts ReadOptions) (Chunk, error) {
	var c Chunk
	o, err := decimal(opts.Offset)
	if err != nil {
		return c, err
	}
	limit := opts.Limit
	if limit == 0 {
		limit = ChunkLimit
	}
	if limit < 1 || limit > ChunkLimit {
		return c, errors.New("Artifact byte limit must be 1–65536.")
	}
	m, f, n, err := s.open(ctx, id, false)
	if err != nil {
		return c, err
	}
	defer f.Close()
	through := n
	if opts.Through != nil {
		through, err = decimal(*opts.Through)
		if err != nil {
			return c, err
		}
	}
	if o > through || through > n {
		return c, errors.New("Artifact byte range exceeds its durable snapshot.")
	}
	count := min(int64(limit), through-o)
	var b []byte
	if m.Closed && m.Completeness == "complete" {
		proof, proofErr := s.completeProof(ctx, m, f, n)
		if proofErr != nil {
			return c, proofErr
		}
		b, err = verifiedRange(f, proof, n, o, count)
		if err != nil {
			return c, err
		}
	} else {
		b = make([]byte, int(count))
		if count > 0 {
			if _, err = f.ReadAt(b, o); err != nil {
				return c, ErrCorrupt
			}
		}
	}
	return Chunk{ArtifactID: id, Offset: strconv.FormatInt(o, 10), NextOffset: strconv.FormatInt(o+count, 10), Through: strconv.FormatInt(through, 10), BytesBase64: base64.StdEncoding.EncodeToString(b), EOF: o+count == through, StreamClosed: m.Closed, Completeness: m.Completeness, SHA256: m.SHA256}, nil
}

func wrapFault(err error) error {
	// The metadata writer returns these exact sentinels only when no commit was
	// attempted. A joined I/O or commit fault must retain its durability marker.
	if err == nil || err == context.Canceled || err == context.DeadlineExceeded {
		return err
	}
	return fmt.Errorf("%w %w", ErrDurability, err)
}
