// Package process owns one bounded native process scope, not an OS security sandbox.
package process

import (
	"context"
	"io"
)

type NativeSpec struct {
	Executable string
	Args       []string
	Cwd        string
	Env        []string // nil inherits the owner environment; values are never logged or persisted here.
	Stdin      []byte
	Stdout     io.Writer
	Stderr     io.Writer
}

type RootOutcome struct {
	Known      bool
	ExitCode   int
	Err        error
	ObservedAt string
}

type StreamFault struct{ Stream, Message string }

const StdinNotStarted = "not_started"
const StdinComplete = "complete"
const StdinIncomplete = "incomplete"

type CleanupResult struct {
	StdinState         string
	StdinBytesWritten  int64 // Completed pipe-write evidence; it does not prove worker consumption.
	ExpectedInputBytes int64
	StdinError         bool // A known error; incomplete without this flag may still be unsettled.
	Verified           bool
	Scope              string
	Detail             string
	Root               *RootOutcome
	StdoutDrained      bool
	StderrDrained      bool
	StdoutError        bool
	StderrError        bool
}

type nativeOwner interface {
	start() error
	cleanup(context.Context) (bool, string, string)
	close() error
}
