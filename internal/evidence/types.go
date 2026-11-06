// Package evidence retains private, byte-exact execution artifacts.
package evidence

import (
	"context"
	"errors"
)

const (
	StdoutLimit int64 = 64 << 20
	StderrLimit int64 = 16 << 20
	LineLimit         = 1 << 20
	ChunkLimit        = 64 << 10
)

var (
	ErrCorrupt    = errors.New("Retained evidence is missing, changed or redirected.")
	ErrLimit      = errors.New("The retained output reached its byte limit.")
	ErrDurability = errors.New("Evidence durability could not be confirmed.")
	ErrClosed     = errors.New("The evidence stream is closed.")
)

// Metadata's storage reference and physical identity are private persistence fields.
// Never serialize them directly to a public API.
type Metadata struct {
	ID           string  `json:"id"`
	AttemptID    string  `json:"attemptId"`
	Kind         string  `json:"kind"`
	ByteLength   string  `json:"byteLength"`
	Closed       bool    `json:"closed"`
	Completeness string  `json:"completeness"`
	SHA256       *string `json:"sha256"`
	CreatedAt    string  `json:"createdAt"`
	RelativePath string  `json:"-"`
	Identity     string  `json:"-"`
}

// Sink commits metadata only after the corresponding bytes are durable.
// An uncertain or failed commit must fence further execution in the caller.
type Sink interface {
	SaveArtifact(context.Context, Metadata) error
	LoadArtifact(context.Context, string) (Metadata, error)
}

type ReadOptions struct {
	Offset  string
	Through *string
	Limit   int
}

type Chunk struct {
	ArtifactID   string  `json:"artifactId"`
	Offset       string  `json:"offset"`
	NextOffset   string  `json:"nextOffset"`
	Through      string  `json:"through"`
	BytesBase64  string  `json:"bytesBase64"`
	EOF          bool    `json:"eof"`
	StreamClosed bool    `json:"streamClosed"`
	Completeness string  `json:"completeness"`
	SHA256       *string `json:"sha256"`
}
