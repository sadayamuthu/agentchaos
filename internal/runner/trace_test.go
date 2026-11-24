package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	switchyard "github.com/sadayamuthu/switchyard"
)

func writeLines(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadTraceLargeLine(t *testing.T) {
	big, _ := json.Marshal(strings.Repeat("x", 200<<10))
	e := switchyard.Event{Hop: "crm", Kind: switchyard.KindResponse, Result: big}
	b, _ := json.Marshal(e)
	p := writeLines(t, string(b), string(b)) // the last line has no newline
	evs, err := readTrace(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || len(evs[1].Result) != len(big) {
		t.Fatalf("got %d events", len(evs))
	}
}

func TestReadTraceMissing(t *testing.T) {
	evs, err := readTrace(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil || len(evs) != 0 {
		t.Fatalf("%v %v", evs, err)
	}
}

func TestReadTraceMalformed(t *testing.T) {
	if _, err := readTrace(writeLines(t, `{"hop":"crm"}`, `{not json`, `{"hop":"crm"}`)); err == nil {
		t.Fatal("want error for malformed line")
	}
}

func TestMergeEventsStable(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := []switchyard.Event{{Hop: "a", ID: "1", Time: t0.Add(2)}, {Hop: "a", ID: "2", Time: t0.Add(1)}}
	b := []switchyard.Event{{Hop: "b", ID: "1", Time: t0.Add(1)}, {Hop: "b", ID: "2", Time: t0}}
	got := mergeEvents([][]switchyard.Event{a, b})
	var order []string
	for _, e := range got {
		order = append(order, e.Hop+e.ID)
	}
	if strings.Join(order, ",") != "b2,a2,b1,a1" {
		t.Fatalf("order %v", order)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"CRM Error!": "crm-error", "a__b--c": "a-b-c", "!!!": "experiment", "x1": "x1"} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}
