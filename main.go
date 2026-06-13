// squizy — load-test OpenAI-compatible chat endpoints and report honest
// token throughput under simulated concurrent users.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"squizy/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cli.Main(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "squizy: %v\n", err)
		os.Exit(1)
	}
}
