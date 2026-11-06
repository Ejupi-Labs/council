package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/Ejupi-Labs/council/v2/internal/cli"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
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
	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
