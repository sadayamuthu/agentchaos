package report_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
	"github.com/sadayamuthu/agentchaos/internal/grade"
	"github.com/sadayamuthu/agentchaos/internal/report"
	"github.com/sadayamuthu/agentchaos/internal/runner"
)

func file(threshold float64) *experiment.File {
	return &experiment.File{
		Path: "/x/exp.yaml", Runs: 5, PassThreshold: threshold,
		Experiments: []experiment.Experiment{{Name: "crm.lookup timeout"}, {Name: "orders.refund error 500"}},
	}
}

func run(name string, i int, status, reason string, checks ...grade.Check) runner.RunResult {
	return runner.RunResult{Experiment: name, Index: i, Status: status, Reason: reason, Checks: checks, Duration: 1500 * time.Millisecond}
}

func results() []runner.RunResult {
	bad := grade.Check{Name: "backoff", Detail: "no backoff between retries"}
	other := grade.Check{Name: "max_calls", Detail: "called 41 times, max 3"}
	var rs []runner.RunResult
	for i := 1; i <= 5; i++ {
		if i == 5 {
			rs = append(rs, run("crm.lookup timeout", i, "fail", "", grade.Check{Name: "must_call", Detail: "never called x"}))
		} else {
			rs = append(rs, run("crm.lookup timeout", i, "pass", "", grade.Check{Name: "a", Pass: true, Detail: "fine"}))
		}
	}
	rs = append(rs,
		run("orders.refund error 500", 1, "fail", "", other, bad),
		run("orders.refund error 500", 2, "fail", "", bad),
		run("orders.refund error 500", 3, "fail", "", bad),
		run("orders.refund error 500", 4, "invalid", "fault-not-triggered"),
		run("orders.refund error 500", 5, "pass", ""),
	)
	return rs
}

func TestTableGolden(t *testing.T) {
	var buf bytes.Buffer
	report.Table(&buf, file(0.8), results(), "out/report.json")
	want, err := os.ReadFile("testdata/table.golden")
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != string(want) {
		t.Fatalf("table mismatch\ngot:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestExitCode(t *testing.T) {
	if c := report.ExitCode(file(0.8), results()[:5]); c != 1 { // second experiment has no runs
		t.Fatalf("missing experiment: %d", c)
	}
	f := &experiment.File{Runs: 5, PassThreshold: 0.8, Experiments: []experiment.Experiment{{Name: "crm.lookup timeout"}}}
	if c := report.ExitCode(f, results()[:5]); c != 0 {
		t.Fatalf("4/5 at 0.8: %d", c)
	}
	f.PassThreshold = 1.0
	if c := report.ExitCode(f, results()[:5]); c != 1 {
		t.Fatalf("4/5 at 1.0: %d", c)
	}
}

func TestWriteJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.json")
	if err := report.WriteJSON(p, file(0.8), results()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	var got struct {
		Version     int    `json:"version"`
		File        string `json:"file"`
		Experiments []struct {
			Name     string  `json:"name"`
			PassRate float64 `json:"pass_rate"`
			Runs     []struct {
				Index      int     `json:"index"`
				Status     string  `json:"status"`
				DurationMS float64 `json:"duration_ms"`
				Checks     []struct {
					Name   string `json:"name"`
					Detail string `json:"detail"`
				} `json:"checks"`
			} `json:"runs"`
		} `json:"experiments"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.File != "/x/exp.yaml" || len(got.Experiments) != 2 {
		t.Fatalf("%+v", got)
	}
	e := got.Experiments[0]
	if e.Name != "crm.lookup timeout" || e.PassRate != 0.8 || len(e.Runs) != 5 || e.Runs[0].DurationMS != 1500 || e.Runs[0].Checks[0].Name != "a" {
		t.Fatalf("%+v", e)
	}
	if got.Experiments[1].PassRate != 0.2 {
		t.Fatalf("%+v", got.Experiments[1])
	}
}
