// Package engine coordinates one durable, explicitly requested authoring attempt.
package engine

import (
	"context"
	"sync"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/store"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type Config struct {
	OwnerInstanceID string
	Resolve         adapters.Resolver
}

type Engine struct {
	// control orders accepted user controls against the last durable launch guard
	// and native Start. No external operation runs inside a SQL transaction.
	control      sync.Mutex
	db           *store.Store
	lock         *localstate.Lock
	config       Config
	evidence     *evidence.Store
	workspace    *workspace.Manager
	ctx          context.Context
	cancel       context.CancelFunc
	wake         chan struct{}
	done         chan struct{}
	shutdown     chan struct{}
	shutdownOnce sync.Once
	closeOnce    sync.Once
	closeErr     error
	watchEnd     chan struct{}
	activeID     string
	activeCancel context.CancelFunc
	workers      sync.WaitGroup
	fenced       bool
	stopWatches  map[string]struct{}
}
