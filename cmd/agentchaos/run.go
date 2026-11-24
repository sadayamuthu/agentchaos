package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
	"github.com/sadayamuthu/agentchaos/internal/report"
	"github.com/sadayamuthu/agentchaos/internal/runner"
)

const runUsage = "usage: agentchaos run FILE [--out DIR] [--json PATH]"

// runCmd implements "agentchaos run". It exits 0 when every experiment meets
// the pass threshold, 1 when one does not or the run was interrupted, and 2
// on usage or configuration errors.
func runCmd(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agentchaos run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, runUsage)
		fs.PrintDefaults()
	}
	out := fs.String("out", "", "output `DIR` (default ./agentchaos-out/<timestamp>)")
	jsonPath := fs.String("json", "", "JSON report `PATH` (default <out>/report.json)")

	// Accept the flags before or after FILE.
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(positional) != 1 {
		fmt.Fprintln(stderr, runUsage)
		return 2
	}

	f, err := experiment.Load(positional[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentchaos: %v\n", err)
		return 2
	}
	self, err := os.Executable()
	if err == nil {
		self, err = filepath.EvalSymlinks(self)
	}
	if err != nil {
		fmt.Fprintf(stderr, "agentchaos: finding own executable: %v\n", err)
		return 2
	}
	dir := *out
	if dir == "" {
		dir = filepath.Join("agentchaos-out", time.Now().Format("20060102-150405"))
	}
	// The agent and its proxies run in other directories, so every path
	// derived from these must be absolute.
	if dir, err = filepath.Abs(dir); err != nil {
		fmt.Fprintf(stderr, "agentchaos: %v\n", err)
		return 2
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "agentchaos: %v\n", err)
		return 2
	}
	reportPath := *jsonPath
	if reportPath == "" {
		reportPath = filepath.Join(dir, "report.json")
	}
	if reportPath, err = filepath.Abs(reportPath); err != nil {
		fmt.Fprintf(stderr, "agentchaos: %v\n", err)
		return 2
	}

	r := &runner.Runner{File: f, Self: self, WorkDir: dir}
	results := r.RunAll(ctx)

	if err := report.WriteJSON(reportPath, f, results); err != nil {
		fmt.Fprintf(stderr, "agentchaos: writing report: %v\n", err)
		return 2
	}
	report.Table(stdout, f, results, reportPath)
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "agentchaos: interrupted")
		return 1
	}
	return report.ExitCode(f, results)
}
