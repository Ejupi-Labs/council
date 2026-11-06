package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

const proofCacheEntries = 128
const proofCacheBytes = 4 << 20

type rangeProof struct {
	identity, hash, length string
	chunks                 [][sha256.Size]byte
}

// completeProof derives chunk digests in the same pass that validates the retained
// whole-artifact hash. Cache hits prove requested ranges, not unrelated current bytes.
func (s *Store) completeProof(ctx context.Context, m Metadata, f *os.File, n int64) (rangeProof, error) {
	s.proofMu.Lock()
	cached, ok := s.proofs[m.ID]
	s.proofMu.Unlock()
	if ok && m.SHA256 != nil && cached.identity == m.Identity && cached.hash == *m.SHA256 && cached.length == m.ByteLength {
		return cached, nil
	}
	if m.SHA256 == nil {
		return rangeProof{}, ErrCorrupt
	}
	p := rangeProof{identity: m.Identity, hash: *m.SHA256, length: m.ByteLength}
	h := sha256.New()
	buf := make([]byte, ChunkLimit)
	for offset := int64(0); offset < n; {
		if err := ctx.Err(); err != nil {
			return rangeProof{}, err
		}
		chunk := buf[:min(int64(len(buf)), n-offset)]
		if _, err := f.ReadAt(chunk, offset); err != nil {
			return rangeProof{}, ErrCorrupt
		}
		h.Write(chunk)
		p.chunks = append(p.chunks, sha256.Sum256(chunk))
		offset += int64(len(chunk))
	}
	if hex.EncodeToString(h.Sum(nil)) != p.hash {
		return rangeProof{}, ErrCorrupt
	}
	s.proofMu.Lock()
	defer s.proofMu.Unlock()
	if s.proofs == nil {
		s.proofs = make(map[string]rangeProof)
	}
	bytesUsed := len(p.chunks) * sha256.Size
	if bytesUsed > proofCacheBytes {
		return p, nil
	}
	for _, proof := range s.proofs {
		bytesUsed += len(proof.chunks) * sha256.Size
	}
	for len(s.proofs) >= proofCacheEntries || bytesUsed > proofCacheBytes {
		for key, proof := range s.proofs {
			bytesUsed -= len(proof.chunks) * sha256.Size
			delete(s.proofs, key)
			break
		}
	}
	s.proofs[m.ID] = p
	return p, nil
}

func verifiedRange(f *os.File, p rangeProof, n, offset, count int64) ([]byte, error) {
	result := make([]byte, int(count))
	if count == 0 {
		return result, nil
	}
	first := offset / ChunkLimit
	last := (offset + count - 1) / ChunkLimit
	for index := first; index <= last; index++ {
		start := index * ChunkLimit
		end := min(start+ChunkLimit, n)
		chunk := make([]byte, int(end-start))
		if _, err := f.ReadAt(chunk, start); err != nil {
			return nil, ErrCorrupt
		}
		if index >= int64(len(p.chunks)) || sha256.Sum256(chunk) != p.chunks[index] {
			return nil, ErrCorrupt
		}
		from := max(offset, start)
		through := min(offset+count, end)
		copy(result[from-offset:through-offset], chunk[from-start:through-start])
	}
	return result, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(b)
}

// CopyTo validates the bytes actually copied in one linear pass. Destinations must
// remain isolated until this returns successfully; a failure can leave partial bytes.
func (s *Store) CopyTo(ctx context.Context, id string, destination io.Writer, maxBytes int64) (Metadata, error) {
	m, f, n, err := s.open(ctx, id, false)
	if err != nil {
		return m, err
	}
	defer f.Close()
	if !m.Closed || m.Completeness != "complete" || m.SHA256 == nil || n > maxBytes {
		return m, ErrCorrupt
	}
	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, h), contextReader{ctx, io.LimitReader(f, n+1)})
	if err != nil {
		return m, err
	}
	if written != n || hex.EncodeToString(h.Sum(nil)) != *m.SHA256 {
		return m, ErrCorrupt
	}
	return m, nil
}
