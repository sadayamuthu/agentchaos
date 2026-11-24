// Command agentchaos is chaos engineering for AI agents.
//
// Usage:
//
//	agentchaos proxy --experiment FILE --name EXPERIMENT --server SERVER [--trace FILE] -- <cmd> [args...]
//	agentchaos run
//	agentchaos version
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/sadayamuthu/agentchaos/internal/proxycmd"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = "usage: agentchaos <proxy|run|version> [args]"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "proxy":
		return proxycmd.Run(ctx, args[1:], stdin, stdout, stderr)
	case "run":
		fmt.Fprintln(stderr, "agentchaos run: not implemented yet")
		return 2
	case "version":
		fmt.Fprintln(stdout, "agentchaos", version)
		return 0
	case "-h", "--help", "help":
		fmt.Fprintln(stderr, usage)
		return 0
	}
	fmt.Fprintf(stderr, "agentchaos: unknown subcommand %q\n%s\n", args[0], usage)
	return 2
}
