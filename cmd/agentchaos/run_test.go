package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var agentchaosBin, fakeagentBin, testserverBin string

func TestMain(m *testing.M) {
	os.Exit(realMain(m))
}

func realMain(m *testing.M) int {
	dir, err := os.MkdirTemp("", "agentchaos-cli-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	agentchaosBin = filepath.Join(dir, "agentchaos")
	fakeagentBin = filepath.Join(dir, "fakeagent")
	testserverBin = filepath.Join(dir, "testserver")
	for bin, pkg := range map[string]string{agentchaosBin: ".", fakeagentBin: "../../internal/fakeagent", testserverBin: "../../internal/testserver"} {
		if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, out)
			return 1
		}
	}
	return m.Run()
}

func writeExperiment(t *testing.T) string {
	t.Helper()
	argv := []string{fakeagentBin, "--mcp-config", "{mcp_config}", "--prompt", "{prompt}", "--call", "crm.lookup",
		"--max-attempts", "3", "--backoff", "exp", "--base", "50ms", "--on-failure", "report"}
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = fmt.Sprintf("%q", a)
	}
	body := fmt.Sprintf(`agent: {command: [%s]}
servers:
  crm: {command: [%q]}
run_timeout: 30s
experiments:
  - name: CRM Error!
    task: look up the account
    fault: {tool: crm.lookup, type: error, every_call: true, message: crm is down}
    expect:
      max_calls: {tool: crm.lookup, count: 3}
      backoff: true
      output_matches: unable
`, strings.Join(q, ", "), testserverBin)
	p := filepath.Join(t.TempDir(), "exp.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestRunSnapshotsExperiment: the proxies load a snapshot taken when the run
// started, so editing the suite mid-run cannot split injection from grading.
func TestRunSnapshotsExperiment(t *testing.T) {
	dir := t.TempDir()
	orig, edited := filepath.Join(dir, "exp.yaml"), filepath.Join(dir, "edited.yaml")
	// The agent overwrites the suite, changing the fault's message, before
	// it starts the proxies.
	argv := []string{"/bin/sh", "-c", `cp "$EDITED" "$ORIG" && exec "$0" "$@"`,
		fakeagentBin, "--mcp-config", "{mcp_config}", "--call", "crm.lookup", "--on-failure", "report"}
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = fmt.Sprintf("%q", a)
	}
	body := func(msg string) string {
		return fmt.Sprintf(`agent:
  command: [%s]
  env: {ORIG: %q, EDITED: %q}
servers:
  crm: {command: [%q]}
run_timeout: 30s
experiments:
  - name: e
    task: t
    fault: {tool: crm.lookup, type: error, every_call: true, message: %s}
`, strings.Join(q, ", "), orig, edited, testserverBin, msg)
	}
	original := body("original-message")
	for p, b := range map[string]string{orig: original, edited: body("edited-message")} {
		if err := os.WriteFile(p, []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "out")
	stdout, stderr, _ := agentchaos(t, "run", orig, "--out", out)
	data, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatalf("%v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	var rep struct {
		Snapshot    string `json:"experiment_snapshot"`
		Experiments []struct {
			Runs []struct {
				StdoutTail string `json:"stdout_tail"`
			} `json:"runs"`
		} `json:"experiments"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(orig); !strings.Contains(string(b), "edited-message") {
		t.Fatalf("the agent did not edit the original file")
	}
	if len(rep.Experiments) != 1 || len(rep.Experiments[0].Runs) != 1 {
		t.Fatalf("report %s", data)
	}
	if got := rep.Experiments[0].Runs[0].StdoutTail; !strings.Contains(got, "original-message") {
		t.Fatalf("injection did not follow the snapshot: stdout %q", got)
	}
	if want := filepath.Join(out, "experiment.yaml"); rep.Snapshot != want {
		t.Fatalf("experiment_snapshot %q, want %q", rep.Snapshot, want)
	}
	if snap, err := os.ReadFile(rep.Snapshot); err != nil || string(snap) != original {
		t.Fatalf("snapshot %q (%v), want the original bytes", snap, err)
	}
}

func agentchaos(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return agentchaosIn(t, "", args...)
}

// agentchaosIn runs agentchaos with working directory dir ("" for the test's).
func agentchaosIn(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(agentchaosBin, args...)
	cmd.Dir = dir
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return o.String(), e.String(), code
}

func TestRunEndToEnd(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	stdout, stderr, code := agentchaos(t, "run", writeExperiment(t), "--out", out)
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	for _, want := range []string{"EXPERIMENT", "BEHAVIOUR", "RESULT", "CRM Error!", "pass 1/1", "1 of 1 passed. Report: " + filepath.Join(out, "report.json")} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	data, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Version    int
		Provenance struct {
			StartedAt  string `json:"started_at"`
			FinishedAt string `json:"finished_at"`
		}
	}
	if err := json.Unmarshal(data, &rep); err != nil || rep.Version != 1 {
		t.Fatalf("report: %v %s", err, data)
	}
	for _, ts := range []string{rep.Provenance.StartedAt, rep.Provenance.FinishedAt} {
		if !strings.HasSuffix(ts, "Z") {
			t.Errorf("timestamp %q is not UTC", ts)
		}
	}
}

// TestRunRelativeOut: a relative --out is relative to the user's cwd, while
// the agent runs elsewhere, so every path handed to it must be absolute.
func TestRunRelativeOut(t *testing.T) {
	cwd := t.TempDir()
	exp := writeExperiment(t) // in a different directory from cwd
	stdout, stderr, code := agentchaosIn(t, cwd, "run", exp, "--out", "rel/out")
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	data, err := os.ReadFile(filepath.Join(cwd, "rel", "out", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Experiments []struct {
			Runs []struct {
				Status     string   `json:"status"`
				TraceFiles []string `json:"trace_files"`
			} `json:"runs"`
		} `json:"experiments"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Experiments) != 1 || len(rep.Experiments[0].Runs) != 1 {
		t.Fatalf("report %s", data)
	}
	r := rep.Experiments[0].Runs[0]
	if r.Status != "pass" || len(r.TraceFiles) != 1 {
		t.Fatalf("run %+v", r)
	}
	for _, p := range r.TraceFiles {
		if !filepath.IsAbs(p) {
			t.Fatalf("trace path %q is not absolute", p)
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(stdout, "Report: "+filepath.Join(cwd, "rel", "out", "report.json")) {
		t.Fatalf("stdout does not name the absolute report path:\n%s", stdout)
	}
}

func TestRunJSONPath(t *testing.T) {
	dir := t.TempDir()
	j := filepath.Join(dir, "custom.json")
	_, stderr, code := agentchaos(t, "run", writeExperiment(t), "--out", filepath.Join(dir, "o"), "--json", j)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(j); err != nil {
		t.Fatal(err)
	}
}

func TestRunInvalidFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(p, []byte("agent: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := agentchaos(t, "run", p, "--out", t.TempDir())
	if code != 2 || stdout != "" || stderr == "" {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func TestRunAgentNotFound(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "exp.yaml")
	body := fmt.Sprintf(`agent: {command: ["./does-not-exist", "{prompt}"]}
servers:
  crm: {command: [%q]}
experiments:
  - name: e
    task: t
    fault: {tool: crm.lookup, type: error, every_call: true}
`, testserverBin)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	stdout, stderr, code := agentchaos(t, "run", p, "--out", out)
	want := fmt.Sprintf("agentchaos: agent %q not found: ", filepath.Join(dir, "does-not-exist"))
	if code != 2 || stdout != "" || !strings.HasPrefix(stderr, want) {
		t.Fatalf("exit %d stdout %q stderr %q, want exit 2 and stderr starting %q", code, stdout, stderr, want)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("output dir created before the preflight: %v", err)
	}
}

func TestRunUsage(t *testing.T) {
	if _, _, code := agentchaos(t, "run"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}
