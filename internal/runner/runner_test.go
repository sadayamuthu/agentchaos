package runner_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
	"github.com/sadayamuthu/agentchaos/internal/runner"
)

var agentchaosBin, fakeagentBin, testserverBin string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "runner-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	agentchaosBin = filepath.Join(dir, "agentchaos")
	fakeagentBin = filepath.Join(dir, "fakeagent")
	testserverBin = filepath.Join(dir, "testserver")
	for bin, pkg := range map[string]string{agentchaosBin: "../../cmd/agentchaos", fakeagentBin: "../fakeagent", testserverBin: "../testserver"} {
		if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, out)
			return 1
		}
	}
	return m.Run()
}

// fake returns a YAML flow sequence running fakeagent with extra flags.
func fake(extra ...string) string {
	argv := append([]string{fakeagentBin, "--mcp-config", "{mcp_config}", "--prompt", "{prompt}"}, extra...)
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = fmt.Sprintf("%q", a)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// newRunner writes an experiment file whose agent is agentCmd and whose crm
// server is the testserver (with serverEnv as its YAML env mapping, if not
// empty), with the given top-level settings and experiments block, and
// returns a Runner for it.
func newRunner(t *testing.T, agentCmd, serverEnv, settings, experiments string) *runner.Runner {
	t.Helper()
	dir := t.TempDir()
	srv := fmt.Sprintf("{command: [%q]}", testserverBin)
	if serverEnv != "" {
		srv = fmt.Sprintf("{command: [%q], env: %s}", testserverBin, serverEnv)
	}
	body := fmt.Sprintf("agent: {command: %s}\nservers:\n  crm: %s\n%s\nexperiments:\n%s", agentCmd, srv, settings, experiments)
	p := filepath.Join(dir, "exp.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := experiment.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return &runner.Runner{File: f, Self: agentchaosBin, WorkDir: filepath.Join(dir, "work")}
}

// runFile runs newRunner's experiments.
func runFile(t *testing.T, agentCmd, settings, experiments string) []runner.RunResult {
	t.Helper()
	return newRunner(t, agentCmd, "", settings, experiments).RunAll(context.Background())
}

func one(t *testing.T, rs []runner.RunResult) runner.RunResult {
	t.Helper()
	if len(rs) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(rs), rs)
	}
	return rs[0]
}

func checkDetail(t *testing.T, r runner.RunResult, name string) string {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c.Detail
		}
	}
	t.Fatalf("no %q check in %+v", name, r.Checks)
	return ""
}

const errorExperiment = `  - name: CRM Error!
    task: look up the account
    fault: {tool: crm.lookup, type: error, every_call: true, message: crm is down}
    expect:
      max_calls: {tool: crm.lookup, count: 3}
      backoff: true
      output_matches: unable
`

func TestRunPass(t *testing.T) {
	r := one(t, runFile(t, fake("--call", "crm.lookup", "--max-attempts", "3", "--backoff", "exp", "--base", "50ms", "--on-failure", "report"), "run_timeout: 30s", errorExperiment))
	if r.Status != "pass" || r.Reason != "" {
		t.Fatalf("status %s/%s, want pass: %+v", r.Status, r.Reason, r)
	}
	if r.Experiment != "CRM Error!" || r.Index != 1 || r.ExitCode != 0 {
		t.Fatalf("%+v", r)
	}
	if len(r.Checks) != 3 {
		t.Fatalf("checks %+v", r.Checks)
	}
	if !strings.Contains(r.StdoutTail, "unable to complete: ") || !strings.Contains(r.StdoutTail, "crm is down") {
		t.Fatalf("stdout %q", r.StdoutTail)
	}
	if len(r.TraceFiles) != 1 || filepath.Base(r.TraceFiles[0]) != "trace-crm.jsonl" {
		t.Fatalf("trace files %v", r.TraceFiles)
	}
	if got := filepath.Base(filepath.Dir(filepath.Dir(r.TraceFiles[0]))) + "/" + filepath.Base(filepath.Dir(r.TraceFiles[0])); got != "crm-error/1" {
		t.Fatalf("run dir %q, want crm-error/1", got)
	}
}

func TestRunFailNoBackoff(t *testing.T) {
	r := one(t, runFile(t, fake("--call", "crm.lookup", "--max-attempts", "5", "--backoff", "none", "--on-failure", "report"), "run_timeout: 30s", errorExperiment))
	if r.Status != "fail" || r.Reason != "" {
		t.Fatalf("status %s/%s, want fail: %+v", r.Status, r.Reason, r)
	}
	if d := checkDetail(t, r, "max_calls"); d != "called 5 times, max 3" {
		t.Fatalf("max_calls detail %q", d)
	}
}

func TestRunDidNotFinish(t *testing.T) {
	start := time.Now()
	r := one(t, runFile(t, fake("--call", "crm.lookup", "--on-failure", "hang"), "run_timeout: 2s", `  - name: hang
    task: look up the account
    fault: {tool: crm.lookup, type: timeout, every_call: true}
`))
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v, want < 5s", d)
	}
	if r.Status != "fail" || r.Reason != "did-not-finish" {
		t.Fatalf("status %s/%s, want fail/did-not-finish: %+v", r.Status, r.Reason, r)
	}
}

func TestRunDidNotFinishAfterFailure(t *testing.T) {
	// The fault fails the call; the agent then hangs on its own while its
	// MCP proxy and server are still running.
	start := time.Now()
	r := one(t, runFile(t, fake("--call", "crm.lookup", "--on-failure", "hang"), "run_timeout: 2s", `  - name: hang
    task: look up the account
    fault: {tool: crm.lookup, type: error, every_call: true}
`))
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v, want < 5s", d)
	}
	if r.Status != "fail" || r.Reason != "did-not-finish" {
		t.Fatalf("status %s/%s, want fail/did-not-finish: %+v", r.Status, r.Reason, r)
	}
}

func TestRunFaultNotTriggered(t *testing.T) {
	r := one(t, runFile(t, fake("--call", "crm.lookup"), "run_timeout: 30s", `  - name: other
    task: look up the account
    fault: {tool: crm.other, type: error, every_call: true}
`))
	if r.Status != "invalid" || r.Reason != "fault-not-triggered" {
		t.Fatalf("status %s/%s, want invalid/fault-not-triggered: %+v", r.Status, r.Reason, r)
	}
	if !strings.Contains(r.StdoutTail, "enterprise") {
		t.Fatalf("stdout %q", r.StdoutTail)
	}
}

func TestRunAgentMissing(t *testing.T) {
	rs := runFile(t, `["./does-not-exist"]`, "runs: 2\nrun_timeout: 30s", `  - name: first
    task: t
    fault: {tool: crm.lookup, type: error, every_call: true}
  - name: second
    task: t
    fault: {tool: crm.lookup, type: error, every_call: true}
`)
	if len(rs) != 4 {
		t.Fatalf("got %d results, want 4: %+v", len(rs), rs)
	}
	want := []string{"first/1", "first/2", "second/1", "second/2"}
	for i, r := range rs {
		if got := fmt.Sprintf("%s/%d", r.Experiment, r.Index); got != want[i] {
			t.Fatalf("result %d is %s, want %s", i, got, want[i])
		}
		if r.Status != "error" || r.Reason != "agent-error" {
			t.Fatalf("result %d status %s/%s, want error/agent-error: %+v", i, r.Status, r.Reason, r)
		}
	}
}

func TestRunAgentNonZeroNoCalls(t *testing.T) {
	r := one(t, runFile(t, `["/bin/sh", "-c", "echo boom >&2; exit 3"]`, "run_timeout: 30s", `  - name: e
    task: t
    fault: {tool: crm.lookup, type: error, every_call: true}
`))
	if r.Status != "error" || r.Reason != "agent-error" || r.ExitCode != 3 {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.StderrTail, "boom") {
		t.Fatalf("stderr %q", r.StderrTail)
	}
}

func TestRunEnvAndDir(t *testing.T) {
	dir := t.TempDir()
	body := fmt.Sprintf(`agent:
  command: ["/bin/sh", "-c", "pwd; echo chain=$SWITCHYARD_CHAIN_ID; echo foo=$FOO"]
  env: {FOO: bar}
servers:
  crm: {command: [%q]}
runs: 2
experiments:
  - name: e
    task: t
    fault: {tool: crm.lookup, type: error, every_call: true}
`, testserverBin)
	p := filepath.Join(dir, "exp.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := experiment.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	rs := (&runner.Runner{File: f, Self: agentchaosBin, WorkDir: t.TempDir()}).RunAll(context.Background())
	if len(rs) != 2 {
		t.Fatalf("%+v", rs)
	}
	var chains []string
	for _, r := range rs {
		lines := strings.Split(strings.TrimSpace(r.StdoutTail), "\n")
		if len(lines) != 3 {
			t.Fatalf("stdout %q", r.StdoutTail)
		}
		real, _ := filepath.EvalSymlinks(dir)
		if got, _ := filepath.EvalSymlinks(lines[0]); got != real {
			t.Fatalf("cwd %q, want %q", lines[0], dir)
		}
		c := strings.TrimPrefix(lines[1], "chain=")
		if len(c) != 16 {
			t.Fatalf("chain %q", lines[1])
		}
		chains = append(chains, c)
		if lines[2] != "foo=bar" {
			t.Fatalf("env line %q", lines[2])
		}
	}
	if chains[0] == chains[1] {
		t.Fatalf("chain id reused across runs: %s", chains[0])
	}
}

func TestPromptArgvSafe(t *testing.T) {
	prompt := "it's \"$HOME\"\nline2"
	task := strings.ReplaceAll(strings.ReplaceAll(prompt, `"`, `\"`), "\n", `\n`)
	r := one(t, runFile(t, fake("--print-prompt", "--call", "crm.lookup"), "run_timeout: 30s", fmt.Sprintf(`  - name: prompt
    task: "%s"
    fault: {tool: crm.lookup, type: error, every_call: true}
`, task)))
	if !strings.Contains(r.StdoutTail, prompt) {
		t.Fatalf("stdout %q does not contain %q", r.StdoutTail, prompt)
	}
}

func TestBuildArgvClaudePreset(t *testing.T) {
	argv, env := runner.BuildArgv(experiment.Agent{Preset: "claude-code"}, "do it", "/tmp/mcp.json", []string{"crm", "orders"})
	want := []string{"claude", "-p", "do it", "--mcp-config", "/tmp/mcp.json", "--strict-mcp-config", "--allowedTools", "mcp__crm", "mcp__orders"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv %q, want %q", argv, want)
	}
	if env["MCP_TOOL_TIMEOUT"] != "30000" {
		t.Fatalf("env %v", env)
	}
}

func TestBuildArgvCommand(t *testing.T) {
	argv, env := runner.BuildArgv(experiment.Agent{Command: []string{"a", "--p={prompt}", "{mcp_config}", "{allowed_tools}", "x{allowed_tools}"}, Env: map[string]string{"K": "v"}}, "P", "M", []string{"s"})
	want := []string{"a", "--p=P", "M", "mcp__s", "x{allowed_tools}"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv %q, want %q", argv, want)
	}
	if !reflect.DeepEqual(env, map[string]string{"K": "v"}) {
		t.Fatalf("env %v", env)
	}
}

func TestRunReusedWorkDir(t *testing.T) {
	r := newRunner(t, fake("--call", "crm.lookup", "--max-attempts", "5", "--backoff", "none", "--on-failure", "report"), "", "run_timeout: 30s", errorExperiment)
	for i := 0; i < 2; i++ {
		res := one(t, r.RunAll(context.Background()))
		if res.Status != "fail" || res.Reason != "" {
			t.Fatalf("pass %d: status %s/%s, want fail: %+v", i+1, res.Status, res.Reason, res)
		}
		if d := checkDetail(t, res, "max_calls"); d != "called 5 times, max 3" {
			t.Fatalf("pass %d: max_calls detail %q", i+1, d)
		}
		if strings.Count(res.StdoutTail, "unable to complete") != 1 {
			t.Fatalf("pass %d: stdout %q", i+1, res.StdoutTail)
		}
	}
}

func TestRunCancelled(t *testing.T) {
	r := newRunner(t, fake("--call", "crm.lookup", "--on-failure", "hang"), "", "run_timeout: 30s", `  - name: hang
    task: look up the account
    fault: {tool: crm.lookup, type: error, every_call: true}
  - name: never
    task: look up the account
    fault: {tool: crm.lookup, type: error, every_call: true}
`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trace := filepath.Join(r.WorkDir, "hang", "1", "trace-crm.jsonl")
	go func() {
		// Cancel once the agent has made a call (its trace has a response).
		for ctx.Err() == nil {
			if b, _ := os.ReadFile(trace); strings.Contains(string(b), `"kind":"response"`) {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	start := time.Now()
	rs := r.RunAll(ctx)
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("took %v", d)
	}
	res := one(t, rs)
	if res.Status != "error" || res.Reason != "cancelled" {
		t.Fatalf("status %s/%s, want error/cancelled: %+v", res.Status, res.Reason, res)
	}
}

func TestRunTimeoutKillsGrandchildren(t *testing.T) {
	// The proxy the agent starts runs a shell that leaves a sleep behind
	// (which no stdin EOF or proxy shutdown would stop) and then becomes the
	// testserver, which writes its own pid. Both are in the agent's process
	// group, so the kill at the wall-clock limit must reach them.
	r := newRunner(t, fake("--call", "crm.lookup", "--on-failure", "hang"), "", "run_timeout: 2s", `  - name: hang
    task: look up the account
    fault: {tool: crm.lookup, type: timeout, every_call: true}
`)
	runDir := filepath.Join(r.WorkDir, "hang", "1")
	serverPID, sleepPID := filepath.Join(runDir, "server.pid"), filepath.Join(runDir, "sleep.pid")
	r.File.Servers["crm"] = experiment.Server{
		Command: []string{"/bin/sh", "-c", `sleep 300 </dev/null >/dev/null 2>&1 & echo $! > "$SLEEP_PIDFILE"; exec "$0"`, testserverBin},
		Env:     map[string]string{"TESTSERVER_PIDFILE": serverPID, "SLEEP_PIDFILE": sleepPID},
	}
	res := one(t, r.RunAll(context.Background()))
	if res.Status != "fail" || res.Reason != "did-not-finish" {
		t.Fatalf("status %s/%s: %+v", res.Status, res.Reason, res)
	}
	for _, f := range []string{serverPID, sleepPID} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var pid int
		if _, err := fmt.Sscan(string(b), &pid); err != nil || pid <= 0 {
			t.Fatalf("%s: %q: %v", f, b, err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			err := syscall.Kill(pid, 0)
			if err == syscall.ESRCH {
				break
			}
			if time.Now().After(deadline) {
				_ = syscall.Kill(pid, syscall.SIGKILL)
				t.Fatalf("pid %d from %s still exists after the run: kill(pid, 0) = %v", pid, filepath.Base(f), err)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
