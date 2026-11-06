// Package adapters interprets supported native protocols without granting prose authority.
package adapters

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const CodexContract = "codex-exec-jsonl/002-v1"
const ClaudeContract = "claude-print-stream-json/003-v1"

// Parser finalizes only protocol evidence; native root/input/drain/cleanup proof
// remains the caller's responsibility.
type Parser interface {
	Write([]byte) (int, error)
	Finish() error
	Result() ProtocolResult
	Failed() <-chan struct{}
}

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
	// FinalResponse is native terminal text; nil is absent, while an empty value
	// can accompany a useful authored candidate. Codex retains its file import.
	FinalResponse *string
	// StructuredOutput is the exact native role result object. Author output is unchanged.
	StructuredOutput []byte
	Terminal         bool
	Succeeded        bool
	Failure          *domain.PrimaryFailure
	Observations     domain.TargetObservations
}
