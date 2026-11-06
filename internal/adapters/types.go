// Package adapters interprets supported native protocols without granting prose authority.
package adapters

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const CodexContract = "codex-exec-jsonl/002-v1"

type Launch struct {
	Executable   string
	Args         []string
	Prompt       []byte
	Observations domain.TargetObservations
}

type Resolver func(context.Context, domain.ExecutionPins) (string, error)

// Event contains only bounded protocol metadata. Full text remains in raw artifacts.
type Event struct {
	Kind         string
	Metadata     []byte
	Observations *domain.TargetObservations
	Failure      *domain.PrimaryFailure
}

type ProtocolResult struct {
	Terminal     bool
	Succeeded    bool
	Failure      *domain.PrimaryFailure
	Observations domain.TargetObservations
}
