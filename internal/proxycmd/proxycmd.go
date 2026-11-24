// Package proxycmd implements the agentchaos proxy subcommand: a switchyard
// MCP proxy wrapping one upstream server and applying an experiment's fault.
package proxycmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/sadayamuthu/switchyard"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
	"github.com/sadayamuthu/agentchaos/internal/faults"
)

const usage = "usage: agentchaos proxy --experiment FILE --name EXPERIMENT --server SERVER [--trace FILE] [--log FILE] -- <cmd> [args...]"

// eofReader records that the wrapped reader reached EOF.
type eofReader struct {
	r   io.Reader
	eof atomic.Bool
}

func (e *eofReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if errors.Is(err, io.EOF) {
		e.eof.Store(true)
	}
	return n, err
}

// Run executes the proxy subcommand with args (the arguments after "proxy")
// and returns the process exit code: 0 on a clean end, 1 on a load or runtime
// error, 2 on bad usage. stdout carries protocol messages only.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agentchaos proxy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, usage)
		fs.PrintDefaults()
	}
	expPath := fs.String("experiment", "", "experiment `FILE`")
	name := fs.String("name", "", "name of the experiment in FILE")
	server := fs.String("server", "", "key of the server this proxy fronts")
	trace := fs.String("trace", "", "append JSONL trace events to `FILE`")
	logPath := fs.String("log", "", "append the proxy's diagnostics and the upstream's stderr to `FILE`")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *logPath != "" {
		lf, err := os.OpenFile(*logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintln(stderr, "agentchaos proxy:", err)
			return 1
		}
		defer lf.Close()
		stderr = lf
		// switchyard.RunCommand gives the upstream this process's
		// os.Stderr, so pointing it at the log captures the server's
		// stderr too.
		prev := os.Stderr
		os.Stderr = lf
		defer func() { os.Stderr = prev }()
	}
	if *expPath == "" || *name == "" || *server == "" {
		fmt.Fprintln(stderr, "agentchaos proxy: --experiment, --name and --server are required")
		fs.Usage()
		return 2
	}
	argv := fs.Args()
	if len(argv) == 0 {
		fmt.Fprintln(stderr, "agentchaos proxy: no upstream command after --")
		fs.Usage()
		return 2
	}

	file, err := experiment.Load(*expPath)
	if err != nil {
		fmt.Fprintln(stderr, "agentchaos proxy:", err)
		return 1
	}
	var exp *experiment.Experiment
	for i := range file.Experiments {
		if file.Experiments[i].Name == *name {
			exp = &file.Experiments[i]
			break
		}
	}
	if exp == nil {
		fmt.Fprintf(stderr, "agentchaos proxy: unknown experiment %q in %s\n", *name, file.Path)
		return 2
	}
	if _, ok := file.Servers[*server]; !ok {
		fmt.Fprintf(stderr, "agentchaos proxy: unknown server %q in %s\n", *server, file.Path)
		return 2
	}

	// A hook is always installed: switchyard emits events only for hooked
	// requests, so this is what makes every call appear in the trace.
	hook := faults.NewPassThrough()
	if exp.Fault.Server() == *server {
		hook = faults.New(exp.Fault)
	}
	opts := switchyard.Options{Name: *server, Hooks: []switchyard.Hook{hook}, ChainID: switchyard.ChainIDFromEnv()}
	if *trace != "" {
		f, err := os.OpenFile(*trace, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintln(stderr, "agentchaos proxy:", err)
			return 1
		}
		defer f.Close()
		opts.Sink = switchyard.NewJSONLSink(f)
	}

	in := &eofReader{r: stdin}
	err = switchyard.RunCommand(ctx, switchyard.New(opts), argv, in, stdout)
	// The upstream exiting on its own after the client was done is not a
	// failure.
	if err != nil && !(errors.Is(err, switchyard.ErrUpstreamClosed) && in.eof.Load()) {
		fmt.Fprintf(stderr, "agentchaos proxy %s: %v\n", *server, err)
		return 1
	}
	return 0
}
