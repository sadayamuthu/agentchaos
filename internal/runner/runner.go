// Package runner runs AgentChaos experiments: for each run it writes an MCP
// config wrapping every server in agentchaos proxy, runs the agent under a
// wall-clock limit, then merges and grades the proxies' traces.
package runner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
	"github.com/sadayamuthu/agentchaos/internal/grade"
	switchyard "github.com/sadayamuthu/switchyard"
)

// Statuses and reasons of a RunResult.
const (
	StatusPass    = "pass"
	StatusFail    = "fail"
	StatusInvalid = "invalid"
	StatusError   = "error"

	ReasonDidNotFinish      = "did-not-finish"
	ReasonFaultNotTriggered = "fault-not-triggered"
	ReasonAgentError        = "agent-error"
	ReasonCancelled         = "cancelled"
)

// tailSize is how much of the agent's stdout and stderr a RunResult keeps.
const tailSize = 4 << 10

// RunResult is the outcome of one run of one experiment.
type RunResult struct {
	Experiment string
	Index      int
	// Status is pass, fail, invalid or error.
	Status string
	// Reason is did-not-finish, fault-not-triggered, agent-error, cancelled
	// or empty.
	Reason                 string
	Checks                 []grade.Check
	ExitCode               int
	Duration               time.Duration
	TraceFiles             []string
	StdoutTail, StderrTail string
}

// Runner runs the experiments of File.
type Runner struct {
	File *experiment.File
	// Self is the path to the agentchaos binary, used as each server's proxy.
	Self string
	// WorkDir holds one directory per run: <exp-slug>/<index>/.
	WorkDir string
}

// RunAll runs the experiments in order, each File.Runs times, sequentially.
// It stops early, returning the results so far, when ctx is done.
func (r *Runner) RunAll(ctx context.Context) []RunResult {
	var out []RunResult
	used := map[string]bool{}
	for _, exp := range r.File.Experiments {
		s := slug(exp.Name)
		for n := 2; used[s]; n++ {
			s = slug(exp.Name) + "-" + strconv.Itoa(n)
		}
		used[s] = true
		for i := 1; i <= r.File.Runs; i++ {
			if ctx.Err() != nil {
				return out
			}
			out = append(out, r.run(ctx, exp, filepath.Join(r.WorkDir, s, strconv.Itoa(i)), i))
		}
	}
	return out
}

// mcpServer is one entry of mcp.json's mcpServers.
type mcpServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

func (r *Runner) run(ctx context.Context, exp experiment.Experiment, dir string, index int) RunResult {
	res := RunResult{Experiment: exp.Name, Index: index, ExitCode: -1}
	setupErr := func(err error) RunResult {
		res.Status = StatusError
		res.Checks = append(res.Checks, grade.Check{Name: "setup", Pass: false, Detail: err.Error()})
		return res
	}
	// Every path written into mcp.json and the agent's argv must be
	// absolute: the agent and the proxies do not run in our cwd.
	dir, err := filepath.Abs(dir)
	if err != nil {
		return setupErr(err)
	}
	self := r.Self
	if strings.Contains(self, "/") { // a bare name is left for PATH lookup
		if self, err = filepath.Abs(self); err != nil {
			return setupErr(err)
		}
	}
	expPath, err := filepath.Abs(r.File.Path)
	if err != nil {
		return setupErr(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return setupErr(err)
	}

	servers := make([]string, 0, len(r.File.Servers))
	for name := range r.File.Servers {
		servers = append(servers, name)
	}
	sort.Strings(servers)
	cfg := map[string]map[string]mcpServer{"mcpServers": {}}
	traces := make([]string, len(servers))
	for i, name := range servers {
		srv := r.File.Servers[name]
		traces[i] = filepath.Join(dir, "trace-"+name+".jsonl")
		args := []string{"proxy", "--experiment", expPath, "--name", exp.Name, "--server", name, "--trace", traces[i], "--"}
		env := srv.Env
		if env == nil {
			env = map[string]string{}
		}
		cfg["mcpServers"][name] = mcpServer{Command: self, Args: append(args, srv.Command...), Env: env}
	}
	mcpPath := filepath.Join(dir, "mcp.json")
	stdoutPath, stderrPath := filepath.Join(dir, "stdout.txt"), filepath.Join(dir, "stderr.txt")
	// A reused run directory must not leak the last run's output into this
	// one: the proxies append to their traces.
	for _, p := range append([]string{mcpPath, stdoutPath, stderrPath}, traces...) {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return setupErr(err)
		}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return setupErr(err)
	}
	if err := os.WriteFile(mcpPath, append(data, '\n'), 0o644); err != nil {
		return setupErr(err)
	}

	argv, agentEnv := BuildArgv(r.File.Agent, exp.Task, mcpPath, servers)
	env := os.Environ()
	for _, k := range sortedKeys(agentEnv) {
		env = append(env, k+"="+agentEnv[k])
	}
	env = append(env, switchyard.ChainEnv+"="+newChainID()) // the last value of a key wins

	stdoutF, err := os.Create(stdoutPath)
	if err != nil {
		return setupErr(err)
	}
	defer stdoutF.Close()
	stderrF, err := os.Create(stderrPath)
	if err != nil {
		return setupErr(err)
	}
	defer stderrF.Close()

	start := time.Now()
	ar := runAgent(ctx, argv, filepath.Dir(r.File.Path), env, stdoutF, stderrF, r.File.RunTimeout)
	res.Duration = time.Since(start)
	res.ExitCode = ar.exitCode

	stdout, _ := os.ReadFile(stdoutPath)
	stderr, _ := os.ReadFile(stderrPath)
	if ar.startErr != nil {
		stderr = append(stderr, []byte("agentchaos: starting agent: "+ar.startErr.Error()+"\n")...)
	}
	res.StdoutTail, res.StderrTail = tail(stdout), tail(stderr)

	var lists [][]switchyard.Event
	for _, p := range traces {
		evs, err := readTrace(p)
		if err != nil {
			res.Status = StatusError
			res.Checks = append(res.Checks, grade.Check{Name: "trace", Pass: false, Detail: err.Error()})
			return res
		}
		if _, err := os.Stat(p); err == nil {
			res.TraceFiles = append(res.TraceFiles, p)
		}
		lists = append(lists, evs)
	}
	events := mergeEvents(lists)

	if ar.startErr != nil {
		res.Status, res.Reason = StatusError, ReasonAgentError
		return res
	}
	if ar.cancelled {
		// Stopped from outside: the run says nothing about the agent.
		res.Status, res.Reason = StatusError, ReasonCancelled
		return res
	}
	calls, faulted := 0, false
	for _, e := range events {
		if e.Kind == switchyard.KindRequest && e.Method == "tools/call" {
			calls++
		}
		if e.Kind == switchyard.KindRequest && grade.IsFaulted(e) {
			faulted = true
		}
	}
	res.Checks = grade.Evaluate(exp.Expect, events, string(stdout), ar.exitCode)
	switch {
	// A run killed at the wall-clock limit did not exit on its own, so its
	// exit status is not the agent's.
	case !ar.timedOut && ar.exitCode != 0 && calls == 0:
		res.Status, res.Reason = StatusError, ReasonAgentError
	case ar.timedOut:
		res.Status, res.Reason = StatusFail, ReasonDidNotFinish
	case !faulted:
		res.Status, res.Reason = StatusInvalid, ReasonFaultNotTriggered
	default:
		res.Status = StatusPass
		for _, c := range res.Checks {
			if !c.Pass {
				res.Status = StatusFail
				break
			}
		}
	}
	return res
}

// readTrace reads a JSONL trace of switchyard events. A missing file is zero
// events; a malformed line is an error.
func readTrace(path string) ([]switchyard.Event, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	var evs []switchyard.Event
	for n := 1; ; n++ {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var e switchyard.Event
			if jerr := json.Unmarshal(line, &e); jerr != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, n, jerr)
			}
			evs = append(evs, e)
		}
		if errors.Is(err, io.EOF) {
			return evs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
}

// mergeEvents concatenates lists and stable-sorts the result by Time.
func mergeEvents(lists [][]switchyard.Event) []switchyard.Event {
	var all []switchyard.Event
	for _, l := range lists {
		all = append(all, l...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Time.Before(all[j].Time) })
	return all
}

// slug lowercases name and turns each run of other characters than ASCII
// letters and digits into a single "-", trimmed at the ends.
func slug(name string) string {
	var b strings.Builder
	dash := false
	for _, c := range strings.ToLower(name) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(c)
			continue
		}
		dash = true
	}
	if b.Len() == 0 {
		return "experiment"
	}
	return b.String()
}

func tail(b []byte) string {
	if len(b) > tailSize {
		b = b[len(b)-tailSize:]
	}
	return string(b)
}

func newChainID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error.
	return hex.EncodeToString(b[:])
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
