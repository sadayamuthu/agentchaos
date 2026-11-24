package main

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sadayamuthu/agentchaos/paper/pilot/internal/pilotdata"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestWilson(t *testing.T) {
	lo, hi := wilson(7, 10)
	if !near(lo, 0.3968, 1e-4) || !near(hi, 0.8922, 1e-4) {
		t.Errorf("wilson(7,10) = %v, %v", lo, hi)
	}
	if lo, _ := wilson(0, 10); lo != 0 {
		t.Errorf("wilson(0,10) lo = %v", lo)
	}
	if _, hi := wilson(10, 10); hi != 1 {
		t.Errorf("wilson(10,10) hi = %v", hi)
	}
}

func TestCohenKappa(t *testing.T) {
	var a, b []bool
	add := func(n int, x, y bool) {
		for i := 0; i < n; i++ {
			a, b = append(a, x), append(b, y)
		}
	}
	add(20, true, true)
	add(5, true, false)
	add(10, false, true)
	add(15, false, false)
	po, k, err := cohenKappa(a, b)
	if err != nil || !near(po, 0.7, 1e-9) || !near(k, 0.4, 1e-9) {
		t.Errorf("po=%v kappa=%v err=%v", po, k, err)
	}
	if _, _, err := cohenKappa([]bool{true}, nil); err == nil {
		t.Error("length mismatch: want error")
	}
	if _, _, err := cohenKappa(nil, nil); err == nil {
		t.Error("empty: want error")
	}
}

func loadFixture(t *testing.T) map[string]*cell {
	t.Helper()
	cells, err := loadData("testdata/data", func(s string) { t.Errorf("unexpected warning: %s", s) })
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]*cell{}
	for _, c := range cells {
		m[c.Model+"|"+c.Experiment] = c
	}
	return m
}

func TestCellExcludesErrors(t *testing.T) {
	c := loadFixture(t)["model-a|orders.refund error"]
	if c.Valid != 3 || c.Pass != 2 || c.Excluded["error"] != 1 || c.Excluded["invalid"] != 1 {
		t.Errorf("cell = %+v", c)
	}
	if c.FirstFail["max_calls"] != 1 || c.DistinctSeqs() != 2 {
		t.Errorf("firstFail=%v seqs=%d", c.FirstFail, c.DistinctSeqs())
	}
}

func TestEmptyCellNA(t *testing.T) {
	cells, err := loadData("testdata/data", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := renderRQ1(cells)
	if !strings.Contains(out, "model-b & orders.refund error & 0 & n/a & n/a & n/a & n/a") {
		t.Errorf("rq1 missing n/a row:\n%s", out)
	}
}

func writeTrace(t *testing.T, dir, file string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func call(sec, tool string) string {
	return `{"v":1,"time":"2026-11-24T12:00:` + sec + `Z","hop":"s","kind":"request","id":"1","method":"tools/call","tool":"` + tool + `"}`
}

func TestDistinctSeqs(t *testing.T) {
	root := t.TempDir()
	writeTrace(t, filepath.Join(root, "m", "exp", "1"), "trace-s.jsonl", call("01", "get"), call("02", "get"))
	writeTrace(t, filepath.Join(root, "m", "exp", "2"), "trace-s.jsonl", call("01", "get"))
	report := `{"version":1,"experiments":[{"name":"exp","runs":[
	 {"index":1,"status":"pass","dir":"/gone/exp/1","tool_calls":2},
	 {"index":2,"status":"pass","dir":"C:\\gone\\exp\\2","tool_calls":1}]}]}`
	if err := os.WriteFile(filepath.Join(root, "m", "report.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	cells, err := loadData(root, func(s string) { t.Error(s) })
	if err != nil || len(cells) != 1 || cells[0].DistinctSeqs() != 2 {
		t.Fatalf("cells=%v err=%v", cells, err)
	}
}

func TestToolSequenceOrdersAcrossServers(t *testing.T) {
	got, err := pilotdata.ToolSequence("testdata/data/model-a/orders-refund-error-control/1")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"crm.lookup", "orders.get", "orders.refund"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestRunDirIgnoresAbsolutePrefix(t *testing.T) {
	if got := pilotdata.RunDir("/r", "/Users/x/data/slug/3"); got != filepath.Join("/r", "slug", "3") {
		t.Errorf("got %s", got)
	}
}

func TestMeanSD(t *testing.T) {
	m, sd := meanSD([]float64{2, 2, 3})
	if !near(m, 2.3333, 1e-4) || !near(sd, 0.5774, 1e-4) {
		t.Errorf("mean=%v sd=%v", m, sd)
	}
}
