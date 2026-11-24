package main

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestTexEscape(t *testing.T) {
	if got := tex(`a&b%c_d#e$f`); got != `a\&b\%c\_d\#e\$f` {
		t.Errorf("got %s", got)
	}
}

func TestRQ1FullRow(t *testing.T) {
	cells, err := loadData("testdata/data", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := renderRQ1(cells)
	// calls {2,2,3}: mean 2.33, sample SD 0.58; 1 error and 1 invalid excluded.
	want := `model-a & orders.refund error & 3 & 67 [21, 94] & 2.33 & 0.58 & 2 & 1/1 \\`
	if !strings.Contains(out, want) {
		t.Errorf("missing row %q in:\n%s", want, out)
	}
	if !strings.Contains(out, "sample SD") {
		t.Error("caption must say sample SD")
	}
	if !strings.Contains(out, "model-b & orders.refund error & 0 & n/a & n/a & n/a & n/a & 2/0") {
		t.Errorf("model-b row:\n%s", out)
	}
}

func TestKappaUndefinedRendersNA(t *testing.T) {
	_, k, err := cohenKappa([]bool{true, true}, []bool{true, true})
	if err != nil || !math.IsNaN(k) {
		t.Fatalf("kappa=%v err=%v", k, err)
	}
	if out := renderRQ3(&agreement{tally: tally{N: 2, Po: 1, Kappa: k, BothUnsafe: 2}, Fault: tally{Po: math.NaN(), Kappa: math.NaN()}}); !strings.Contains(out, "$\\kappa$ = n/a") {
		t.Errorf("rq3:\n%s", out)
	}
}

func TestStaleTablesRemoved(t *testing.T) {
	out := t.TempDir()
	for _, f := range []string{"rq2.tex", "rq3.tex"} {
		if err := os.WriteFile(filepath.Join(out, f), []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := run("testdata/data", "testdata/labels-bad", out); err == nil {
		t.Fatal("want label error")
	}
	if fileExists(filepath.Join(out, "rq3.tex")) {
		t.Error("stale rq3.tex survived bad labels")
	}
	if !fileExists(filepath.Join(out, "rq2.tex")) || strings.Contains(readFile(t, filepath.Join(out, "rq2.tex")), "stale") {
		t.Error("rq2.tex should be regenerated")
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunWritesTables(t *testing.T) {
	out := t.TempDir()
	if err := run("testdata/data", "testdata/labels-good", out); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"rq1.tex", "rq2.tex", "rq3.tex"} {
		b, err := os.ReadFile(filepath.Join(out, f))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, `\begin{table`) || !strings.Contains(s, `\end{table`) || strings.Contains(s, "toprule") {
			t.Errorf("%s malformed:\n%s", f, s)
		}
	}
	b, _ := os.ReadFile(filepath.Join(out, "rq2.tex"))
	if !strings.Contains(string(b), "orders.refund error & control & 3 & 2 & 1 & 1") {
		t.Errorf("rq2 control row:\n%s", b)
	}
}

func TestNoLabelsNoRQ3(t *testing.T) {
	out := t.TempDir()
	if err := run("testdata/data", "", out); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(out, "rq3.tex")) {
		t.Error("rq3.tex written without --labels")
	}
}

func TestMain(m *testing.M) {
	stdout = io.Discard
	os.Exit(m.Run())
}

func TestEmptyRunDirWarnsAndSkips(t *testing.T) {
	dir := t.TempDir()
	rep := `{"version":1,"experiments":[{"name":"x","runs":[{"index":1,"status":"pass","dir":"bad","checks":[]}]}]}`
	if err := os.MkdirAll(filepath.Join(dir, "m"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "m", "report.json"), []byte(rep), 0o644); err != nil {
		t.Fatal(err)
	}
	var warns []string
	cells, err := loadData(dir, func(s string) { warns = append(warns, s) })
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "run directory") {
		t.Errorf("warns = %q", warns)
	}
	if len(cells) != 1 || cells[0].DistinctSeqs() != 0 {
		t.Errorf("cells = %+v", cells)
	}
}
