package experiment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadSpecExample(t *testing.T) {
	f, err := Load("testdata/spec.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if f.Runs != 5 || f.RunTimeout != 5*time.Minute || f.PassThreshold != 1.0 {
		t.Fatalf("defaults: %+v", f)
	}
	if !filepath.IsAbs(f.Path) {
		t.Fatalf("path not absolute: %s", f.Path)
	}
	e := f.Experiments[0]
	if e.Fault.Server() != "crm" || e.Fault.ToolName() != "lookup" {
		t.Fatalf("fault split: %+v", e.Fault)
	}
	if e.Fault.ErrorMode != "tool" || e.Fault.Message != "internal error" || e.Fault.Corrupt != "invalid-json" {
		t.Fatalf("fault defaults: %+v", e.Fault)
	}
	if e.Expect.MaxCalls == nil || e.Expect.MaxCalls.Count != 4 || !e.Expect.Backoff {
		t.Fatalf("expect: %+v", e.Expect)
	}
	if f.Agent.Preset != "claude-code" {
		t.Fatalf("agent: %+v", f.Agent)
	}
}

func TestLoadDurations(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.yaml")
	y := `agent: {preset: claude-code}
servers: {s: {command: [x]}}
run_timeout: 90s
experiments:
  - {name: a, task: t, fault: {tool: s.t, type: slow, delay: 300ms, every_call: true}}
`
	if err := os.WriteFile(p, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.RunTimeout != 90*time.Second || f.Experiments[0].Fault.Delay != 300*time.Millisecond {
		t.Fatalf("%+v", f)
	}
}

func TestRelativePaths(t *testing.T) {
	f, err := Load("testdata/sub/x.yaml")
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs("testdata/sub")
	if got := f.Servers["crm"].Command[0]; got != filepath.Join(abs, "crm-server") {
		t.Errorf("server command: %s", got)
	}
	if got := f.Servers["crm"].Command[1]; got != "--flag" {
		t.Errorf("arg changed: %s", got)
	}
	if got := f.Servers["bare"].Command[0]; got != "claude" {
		t.Errorf("bare name changed: %s", got)
	}
	if got := f.Agent.Command[0]; got != filepath.Join(abs, "agent") {
		t.Errorf("agent command: %s", got)
	}
}

func valid() *File {
	return &File{
		Agent:         Agent{Preset: "claude-code"},
		Servers:       map[string]Server{"crm": {Command: []string{"x"}}},
		Runs:          1,
		RunTimeout:    time.Minute,
		PassThreshold: 1,
		Experiments: []Experiment{{
			Name: "a", Task: "t",
			Fault: Fault{Tool: "crm.lookup", Type: "timeout", OnCall: 1},
		}},
	}
}

func TestValidateOK(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(f *File)
		want string
	}{
		{"unknown server", func(f *File) { f.Experiments[0].Fault.Tool = "billing.x" }, `experiments[0].fault.tool: unknown server "billing"`},
		{"bad type", func(f *File) { f.Experiments[0].Fault.Type = "explode" }, "experiments[0].fault.type"},
		{"two triggers", func(f *File) { f.Experiments[0].Fault.EveryCall = true }, "experiments[0].fault: exactly one trigger"},
		{"no trigger", func(f *File) { f.Experiments[0].Fault.OnCall = 0 }, "experiments[0].fault: exactly one trigger"},
		{"slow no delay", func(f *File) { f.Experiments[0].Fault.Type = "slow" }, "experiments[0].fault.delay"},
		{"bad regex", func(f *File) { f.Experiments[0].Expect.OutputMatches = "(" }, "experiments[0].expect.output_matches"},
		{"bad not regex", func(f *File) { f.Experiments[0].Expect.OutputNotMatches = "(" }, "experiments[0].expect.output_not_matches"},
		{"duplicate name", func(f *File) { f.Experiments = append(f.Experiments, f.Experiments[0]) }, "experiments[1].name"},
		{"dotted server", func(f *File) { f.Servers["a.b"] = Server{Command: []string{"x"}} }, `servers["a.b"]`},
		{"must_call unknown", func(f *File) { f.Experiments[0].Expect.MustCall = []string{"zzz.t"} }, "experiments[0].expect.must_call[0]"},
		{"must_not_call unknown", func(f *File) { f.Experiments[0].Expect.MustNotCall = []string{"zzz.t"} }, "experiments[0].expect.must_not_call[0]"},
		{"max_calls unknown", func(f *File) { f.Experiments[0].Expect.MaxCalls = &MaxCalls{Tool: "zzz.t", Count: 1} }, "experiments[0].expect.max_calls.tool"},
		{"runs", func(f *File) { f.Runs = 0 }, "runs"},
		{"threshold", func(f *File) { f.PassThreshold = 1.5 }, "pass_threshold"},
		{"agent both", func(f *File) { f.Agent.Command = []string{"a"} }, "agent"},
		{"agent preset", func(f *File) { f.Agent.Preset = "other" }, "agent.preset"},
		{"probability", func(f *File) { f.Experiments[0].Fault.OnCall = 0; f.Experiments[0].Fault.Probability = 2 }, "experiments[0].fault"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := valid()
			c.mut(f)
			err := f.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q in %v", c.want, err)
			}
		})
	}
}

func TestValidateJoinsAll(t *testing.T) {
	f := valid()
	f.Runs = 0
	f.PassThreshold = 0
	err := f.Validate()
	if err == nil || !strings.Contains(err.Error(), "runs") || !strings.Contains(err.Error(), "pass_threshold") {
		t.Fatalf("got %v", err)
	}
}

// TestLoadDoesNotStatCommands: the proxies load a snapshot of the file from
// the output directory, where relative commands point nowhere.
func TestLoadDoesNotStatCommands(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.yaml")
	y := `agent: {command: [./no/such/agent]}
servers: {s: {command: [./no/such/server]}}
experiments:
  - {name: a, task: t, fault: {tool: s.t, type: error, every_call: true}}
`
	if err := os.WriteFile(p, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err != nil {
		t.Fatal(err)
	}
}
