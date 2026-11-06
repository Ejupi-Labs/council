// Package app implements the shared foundation service used by CLI and HTTP.
package app

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

type Params struct {
	ID    string
	Query map[string]string
	Body  []byte
}
type Result = domain.Result
type Envelope = domain.Envelope
type Error = domain.Error
type Policy = domain.Policy
type Project = domain.Project
type Draft = domain.Draft
type Service struct {
	Execution               ExecutionController
	Store                   *store.Store
	InspectProject          func(context.Context, string, string) (domain.ResolvedProject, error)
	ValidateProjectIdentity func(string, string) error
	InspectAgents           func(context.Context) (domain.DiscoveryResult, error)
}

func New(db *store.Store) *Service                { return newNativeService(db) }
func StrictDecode(body []byte, target any) *Error { return domain.StrictDecode(body, target) }
func ParseRevision(s string) (int64, *Error)      { return domain.ParseRevision(s) }
func NewID() string                               { return domain.NewID() }
