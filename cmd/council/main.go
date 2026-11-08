package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/Ejupi-Labs/council/v2/internal/cli"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/mcphost"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

func main() {
	if handled, code := process.HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	if handled, code := fixture.HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if handled, code := cli.HandleInternal(ctx, os.Args[1:], os.Stderr); handled {
		os.Exit(code)
	}
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		os.Exit(runMCP(ctx, os.Args[2:]))
	}
	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func runMCP(ctx context.Context, args []string) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(os.Stderr, "Usage: council mcp")
		return 2
	}
	owner, err := ownerclient.NewDefault()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Council cannot locate its private state directory.")
		return 1
	}
	server, err := mcphost.NewHost(owner)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Council MCP configuration is unavailable.")
		return 1
	}
	if err = mcphost.Serve(ctx, server, os.Stdin, os.Stdout); err != nil {
		if errors.Is(err, context.Canceled) {
			return 130
		}
		_, _ = fmt.Fprintln(os.Stderr, "Council MCP connection ended with an error.")
		return 1
	}
	return 0
}
