package proxycmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var agentchaosBin, testserverBin string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "proxycmd-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	agentchaosBin = filepath.Join(dir, "agentchaos")
	testserverBin = filepath.Join(dir, "testserver")
	for bin, pkg := range map[string]string{agentchaosBin: "../../cmd/agentchaos", testserverBin: "../testserver"} {
		if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, out)
			return 1
		}
	}
	return m.Run()
}

const experimentYAML = `agent: {command: [echo]}
servers:
  crm: {command: [x]}
  orders: {command: [x]}
experiments:
  - name: crm-error
    task: look up an account
    fault: {tool: crm.lookup, type: error, every_call: true, message: crm is down}
`

func writeExperiment(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "exp.yaml")
	if err := os.WriteFile(p, []byte(experimentYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func proxyArgs(exp, name, server, trace string) []string {
	args := []string{"proxy", "--experiment", exp, "--name", name, "--server", server}
	if trace != "" {
		args = append(args, "--trace", trace)
	}
	return append(args, "--", testserverBin)
}

func connect(t *testing.T, args []string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.Command(agentchaosBin, args...)
	cmd.Stderr = os.Stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callLookup(t *testing.T, cs *mcp.ClientSession) *mcp.CallToolResult {
	t.Helper()
	r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "lookup", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func traceDecisions(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var e struct {
			Hop      string `json:"hop"`
			Decision string `json:"decision"`
		}
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad trace line %q: %v", line, err)
		}
		out = append(out, e.Hop+"/"+e.Decision)
	}
	return out
}

func TestProxyInjectsFault(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "trace.jsonl")
	cs := connect(t, proxyArgs(writeExperiment(t), "crm-error", "crm", trace))
	r := callLookup(t, cs)
	if !r.IsError {
		t.Fatalf("lookup IsError = false, want true: %+v", r)
	}
	if err := cs.Close(); err != nil {
		t.Logf("close: %v", err)
	}
	var found bool
	for _, d := range traceDecisions(t, trace) {
		if d == "crm/block-tool:fault-error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no crm block-tool:fault-error event in trace: %v", traceDecisions(t, trace))
	}
}

func TestProxyOtherServerPassThrough(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "trace.jsonl")
	cs := connect(t, proxyArgs(writeExperiment(t), "crm-error", "orders", trace))
	r := callLookup(t, cs)
	if r.IsError {
		t.Fatalf("lookup IsError = true, want false: %+v", r)
	}
	_ = cs.Close()
	// Even without a fault the call is traced (a hook is always installed).
	var traced bool
	for _, d := range traceDecisions(t, trace) {
		if d == "orders/continue" {
			traced = true
		}
		if strings.Contains(d, "fault") {
			t.Fatalf("pass-through trace has a fault: %v", d)
		}
	}
	if !traced {
		t.Fatalf("no orders/continue event in trace")
	}
}

func TestProxyNoTraceStillPassesThrough(t *testing.T) {
	cs := connect(t, proxyArgs(writeExperiment(t), "crm-error", "orders", ""))
	if r := callLookup(t, cs); r.IsError {
		t.Fatalf("lookup IsError = true: %+v", r)
	}
}

func runBin(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(agentchaosBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.Stdin = strings.NewReader("")
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return code, stdout.String(), stderr.String()
}

func TestProxyUnknownExperiment(t *testing.T) {
	code, stdout, stderr := runBin(t, proxyArgs(writeExperiment(t), "nope", "crm", "")...)
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "nope") {
		t.Fatalf("stderr %q does not name the experiment", stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
}

func TestProxyUsageErrors(t *testing.T) {
	exp := writeExperiment(t)
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"bad flag", []string{"proxy", "--bogus"}, 2},
		{"missing flags", []string{"proxy", "--", testserverBin}, 2},
		{"no command", []string{"proxy", "--experiment", exp, "--name", "crm-error", "--server", "crm"}, 2},
		{"unknown server", proxyArgs(exp, "crm-error", "ghost", ""), 2},
		{"load error", proxyArgs(filepath.Join(t.TempDir(), "missing.yaml"), "crm-error", "crm", ""), 1},
		{"unknown subcommand", []string{"frobnicate"}, 2},
		{"run not implemented", []string{"run"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runBin(t, tt.args...)
			if code != tt.code {
				t.Fatalf("exit = %d, want %d; stderr: %s", code, tt.code, stderr)
			}
			if stdout != "" || stderr == "" {
				t.Fatalf("stdout = %q, stderr = %q; want empty stdout and a diagnostic", stdout, stderr)
			}
		})
	}
}

func TestProxyUnknownServerNamesIt(t *testing.T) {
	_, _, stderr := runBin(t, proxyArgs(writeExperiment(t), "crm-error", "ghost", "")...)
	if !strings.Contains(stderr, "ghost") {
		t.Fatalf("stderr %q does not name the server", stderr)
	}
}

func TestVersion(t *testing.T) {
	code, stdout, _ := runBin(t, "version")
	if code != 0 || !strings.Contains(stdout, "dev") {
		t.Fatalf("exit = %d, stdout = %q", code, stdout)
	}
}

func TestProxyCleanEOFExitsZero(t *testing.T) {
	cmd := exec.Command(agentchaosBin, proxyArgs(writeExperiment(t), "crm-error", "crm", "")...)
	cmd.Stdin = strings.NewReader("")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("exit on clean EOF: %v; stderr: %s", err, stderr.String())
	}
}
