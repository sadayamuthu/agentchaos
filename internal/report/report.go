// Package report renders run results as a terminal table and a JSON report,
// and derives the exit code from them.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
	"github.com/sadayamuthu/agentchaos/internal/runner"
)

// group is one experiment with its runs.
type group struct {
	name string
	runs []runner.RunResult
}

func (g group) passed() int {
	n := 0
	for _, r := range g.runs {
		if r.Status == runner.StatusPass {
			n++
		}
	}
	return n
}

// total is the number of runs the experiment was meant to have, so that an
// interrupted experiment does not look better than it is.
func total(f *experiment.File) int {
	if f.Runs < 1 {
		return 1
	}
	return f.Runs
}

func (g group) rate(f *experiment.File) float64 {
	return float64(g.passed()) / float64(total(f))
}

func (g group) ok(f *experiment.File) bool { return g.rate(f) >= f.PassThreshold }

func groups(f *experiment.File, results []runner.RunResult) []group {
	gs := make([]group, len(f.Experiments))
	idx := map[string]int{}
	for i, e := range f.Experiments {
		gs[i].name = e.Name
		idx[e.Name] = i
	}
	for _, r := range results {
		if i, ok := idx[r.Experiment]; ok {
			gs[i].runs = append(gs[i].runs, r)
		}
	}
	return gs
}

// behaviour describes one run: "ok", else the first failing check's detail,
// else the reason, else the status.
func behaviour(r runner.RunResult) string {
	if r.Status == runner.StatusPass {
		return "ok"
	}
	for _, c := range r.Checks {
		if !c.Pass {
			return c.Detail
		}
	}
	if r.Reason != "" {
		return r.Reason
	}
	return r.Status
}

// behaviour is the most common non-ok behaviour of the runs (the earliest
// wins a tie), or "ok".
func (g group) behaviour() string {
	if len(g.runs) == 0 {
		return "not run"
	}
	counts := map[string]int{}
	var order []string
	for _, r := range g.runs {
		b := behaviour(r)
		if b == "ok" {
			continue
		}
		if counts[b] == 0 {
			order = append(order, b)
		}
		counts[b]++
	}
	best := "ok"
	for _, b := range order {
		if best == "ok" || counts[b] > counts[best] {
			best = b
		}
	}
	return best
}

// Table writes the report table and the summary line naming reportPath.
func Table(w io.Writer, f *experiment.File, results []runner.RunResult, reportPath string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "EXPERIMENT\tBEHAVIOUR\tRESULT")
	passed := 0
	gs := groups(f, results)
	for _, g := range gs {
		verdict := "FAIL"
		if g.ok(f) {
			verdict = "pass"
			passed++
		}
		// A detail can span lines (a server's stderr); a table cell cannot.
		b := strings.Join(strings.Fields(g.behaviour()), " ")
		fmt.Fprintf(tw, "%s\t%s\t%s %d/%d\n", g.name, b, verdict, g.passed(), total(f))
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%d of %d passed. Report: %s\n", passed, len(gs), reportPath)
}

// ExitCode is 0 if every experiment meets the pass threshold, else 1.
func ExitCode(f *experiment.File, results []runner.RunResult) int {
	for _, g := range groups(f, results) {
		if !g.ok(f) {
			return 1
		}
	}
	return 0
}

type jsonCheck struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

type jsonRun struct {
	Index      int         `json:"index"`
	Status     string      `json:"status"`
	Reason     string      `json:"reason"`
	Checks     []jsonCheck `json:"checks"`
	ExitCode   int         `json:"exit_code"`
	DurationMS int64       `json:"duration_ms"`
	TraceFiles []string    `json:"trace_files"`
	StdoutTail string      `json:"stdout_tail"`
	StderrTail string      `json:"stderr_tail"`
}

type jsonExperiment struct {
	Name     string    `json:"name"`
	PassRate float64   `json:"pass_rate"`
	Runs     []jsonRun `json:"runs"`
}

type jsonReport struct {
	Version     int              `json:"version"`
	File        string           `json:"file"`
	Snapshot    string           `json:"experiment_snapshot"`
	Experiments []jsonExperiment `json:"experiments"`
}

// WriteJSON writes the JSON report to path. snapshot is the copy of the
// experiment file the proxies loaded, or "".
func WriteJSON(path string, f *experiment.File, snapshot string, results []runner.RunResult) error {
	rep := jsonReport{Version: 1, File: f.Path, Snapshot: snapshot, Experiments: []jsonExperiment{}}
	for _, g := range groups(f, results) {
		je := jsonExperiment{Name: g.name, PassRate: g.rate(f), Runs: []jsonRun{}}
		for _, r := range g.runs {
			jr := jsonRun{
				Index: r.Index, Status: r.Status, Reason: r.Reason, ExitCode: r.ExitCode,
				DurationMS: r.Duration.Milliseconds(), StdoutTail: r.StdoutTail, StderrTail: r.StderrTail,
				Checks: []jsonCheck{}, TraceFiles: []string{},
			}
			for _, c := range r.Checks {
				jr.Checks = append(jr.Checks, jsonCheck{c.Name, c.Pass, c.Detail})
			}
			jr.TraceFiles = append(jr.TraceFiles, r.TraceFiles...)
			je.Runs = append(je.Runs, jr)
		}
		rep.Experiments = append(rep.Experiments, je)
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
