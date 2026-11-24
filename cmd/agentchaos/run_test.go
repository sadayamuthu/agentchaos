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

func agentchaos(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(agentchaosBin, args...)
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
	var rep struct{ Version int }
	if err := json.Unmarshal(data, &rep); err != nil || rep.Version != 1 {
		t.Fatalf("report: %v %s", err, data)
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

func TestRunUsage(t *testing.T) {
	if _, _, code := agentchaos(t, "run"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}
