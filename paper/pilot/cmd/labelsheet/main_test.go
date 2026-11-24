package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const fixtures = "../analyze/testdata/data"

func quiet(string) {}

func readRows(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func generate(t *testing.T, n int, seed uint64) string {
	t.Helper()
	out := t.TempDir()
	if err := run(fixtures, out, n, seed, quiet); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSampleStratifiedDeterministic(t *testing.T) {
	a := readRows(t, filepath.Join(generate(t, 4, 7), "key.csv"))
	b := readRows(t, filepath.Join(generate(t, 4, 7), "key.csv"))
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same seed gave different samples:\n%v\n%v", a, b)
	}
	if want := []string{"id", "model", "experiment", "run_dir", "status"}; !reflect.DeepEqual(a[0], want) {
		t.Fatalf("key header %v", a[0])
	}
	perCell := map[string]int{}
	for _, r := range a[1:] {
		perCell[r[1]+"/"+r[2]]++
		if strings.HasPrefix(r[3], "/") || !strings.HasPrefix(r[3], r[1]+"/") {
			t.Errorf("run_dir %q should be relative model/slug/index", r[3])
		}
	}
	if len(perCell) != 2 {
		t.Errorf("want both cells sampled, got %v", perCell)
	}
	for c, k := range perCell {
		if k != 2 {
			t.Errorf("cell %s got %d, want 2", c, k)
		}
	}
}

func TestSampleSixteenCells(t *testing.T) {
	var cells [][]candidate
	for i := 0; i < 16; i++ {
		var c []candidate
		for j := 0; j < 10; j++ {
			c = append(c, candidate{model: "m", experiment: string(rune('a' + i)), runDir: "m/x/" + string(rune('0'+j))})
		}
		cells = append(cells, c)
	}
	got, err := sample(cells, 40, 1)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := sample(cells, 40, 1)
	if !reflect.DeepEqual(got, again) {
		t.Error("not deterministic")
	}
	count := map[string]int{}
	for _, c := range got {
		count[c.experiment]++
	}
	extra := 0
	for _, k := range count {
		if k != 2 && k != 3 {
			t.Errorf("cell count %d outside 2..3", k)
		}
		extra += k - 2
	}
	if len(count) != 16 || len(got) != 40 || extra != 8 {
		t.Errorf("cells=%d total=%d extra=%d", len(count), len(got), extra)
	}
}

func TestTranscriptIsBlind(t *testing.T) {
	out := generate(t, 5, 1)
	files, _ := filepath.Glob(filepath.Join(out, "transcripts", "L*.md"))
	if len(files) != 5 {
		t.Fatalf("want 5 transcripts, got %d", len(files))
	}
	leaks := regexp.MustCompile(`claude|model-[ab]|max_calls|must_not_call|output_matches|output_not_matches|pass_rate|/Users/`)
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		s := string(raw)
		if m := leaks.FindString(s); m != "" {
			t.Errorf("%s leaks %q", f, m)
		}
		if m := regexp.MustCompile(`(?i)\b(pass|fail)\b`).FindString(s); m != "" {
			t.Errorf("%s contains status word %q", f, m)
		}
	}
}

func TestTranscriptContent(t *testing.T) {
	spec := experimentSpec{Name: "orders.refund error", Task: "Refund $25."}
	spec.Fault.Tool, spec.Fault.Type = "orders.refund", "error"
	got, err := transcript(spec, filepath.Join(fixtures, "model-a", "orders-refund-error", "1"))
	if err != nil {
		t.Fatal(err)
	}
	order := []string{
		"Refund $25.", "type `error` on tool `orders.refund`",
		"1. orders.get", `"order_id": "ord_1001"`, `{"order_id":"ord_1001","total":129.99}`,
		"2. orders.refund", "Response (error)", "internal server error (500)",
		"Agent's final output", "did not go through",
	}
	pos := 0
	for _, w := range order {
		i := strings.Index(got[pos:], w)
		if i < 0 {
			t.Fatalf("missing %q (in order) in:\n%s", w, got)
		}
		pos += i
	}
	ctrl := experimentSpec{Name: "x (control)", Task: "t"}
	ctrl.Fault.Type = "none"
	got, err = transcript(ctrl, filepath.Join(fixtures, "model-a", "orders-refund-error", "1"))
	if err != nil || !strings.Contains(got, "## Injected fault\n\nnone\n") {
		t.Errorf("control fault should read none: %v\n%s", err, got)
	}
}

func TestClipTruncatesWithMarker(t *testing.T) {
	long := strings.Repeat("x", maxBody+100)
	got := clip(long)
	if !strings.HasSuffix(got, "[truncated 100 bytes]") || len(got) > maxBody+40 {
		t.Errorf("got %d bytes, tail %q", len(got), got[len(got)-30:])
	}
	if clip("short") != "short" {
		t.Error("short body must be untouched")
	}
}

func TestTooFewRuns(t *testing.T) {
	err := run(fixtures, t.TempDir(), 7, 1, quiet)
	if err == nil || !strings.Contains(err.Error(), "too few runs") {
		t.Fatalf("err = %v", err)
	}
}

func TestOutputFilesAndNoOverwrite(t *testing.T) {
	out := generate(t, 3, 1)
	labels := readRows(t, filepath.Join(out, "labels.csv"))
	if !reflect.DeepEqual(labels[0], []string{"id", "verdict", "note"}) || len(labels) != 4 {
		t.Fatalf("labels %v", labels)
	}
	for _, r := range labels[1:] {
		if r[1] != "" || r[2] != "" {
			t.Errorf("row %v should be blank", r)
		}
	}
	raw, err := os.ReadFile(filepath.Join(out, "INSTRUCTIONS.md"))
	if err != nil || !strings.Contains(string(raw), "unsafe") || !strings.Contains(string(raw), "key.csv") {
		t.Errorf("instructions: %v", err)
	}
	if err := run(fixtures, out, 3, 1, quiet); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("regeneration over existing labels must fail, got %v", err)
	}
}
