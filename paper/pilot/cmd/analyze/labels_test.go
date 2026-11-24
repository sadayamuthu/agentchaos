package main

import (
	"math"
	"os"
	"strings"
	"testing"
)

func TestLabelsRejectsBadRows(t *testing.T) {
	_, err := loadAgreement("testdata/labels-bad", "testdata/data")
	if err == nil {
		t.Fatal("want error")
	}
	for _, id := range []string{"r01", "r02", "zzz"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("error %q does not name %s", err, id)
		}
	}
	if strings.Contains(err.Error(), "r03") {
		t.Errorf("valid row r03 flagged: %v", err)
	}
	tex := t.TempDir()
	if err := run("testdata/data", "testdata/labels-bad", tex); err == nil {
		t.Error("run: want error")
	}
	if fileExists(tex + "/rq3.tex") {
		t.Error("rq3.tex written despite bad labels")
	}
}

func TestLabelsAgreement(t *testing.T) {
	a, err := loadAgreement("testdata/labels-good", "testdata/data")
	if err != nil {
		t.Fatal(err)
	}
	// Grader unsafe: r03 (fault arm failed), r05 (control failed max_calls).
	// r04 is a control that failed only output_matches: grader safe.
	// Human unsafe: r02, r03, r05.
	if a.N != 5 || a.BothUnsafe != 2 || a.GraderOnlyUnsafe != 0 || a.HumanOnlyUnsafe != 1 || a.BothSafe != 2 {
		t.Errorf("all: %+v", a.tally)
	}
	if a.Fault.N != 3 || a.Fault.BothUnsafe != 1 || a.Fault.HumanOnlyUnsafe != 1 || a.Fault.BothSafe != 1 || a.Fault.GraderOnlyUnsafe != 0 {
		t.Errorf("fault: %+v", a.Fault)
	}
	if a.Control.N != 2 || a.Control.BothUnsafe != 1 || a.Control.BothSafe != 1 || a.Control.Po != 1 || a.Control.Kappa != 1 {
		t.Errorf("control: %+v", a.Control)
	}
	if math.Abs(a.Po-0.8) > 1e-9 {
		t.Errorf("po=%v", a.Po)
	}
	out := renderRQ3(a)
	for _, want := range []string{"Fault arm: $n$=3", "Control arm: $n$=2", "Control & unsafe & 1 & 0 "} {
		if !strings.Contains(out, want) {
			t.Errorf("rq3 missing %q:\n%s", want, out)
		}
	}
}

func TestLabelsNeedData(t *testing.T) {
	if _, err := loadAgreement("testdata/labels-good", ""); err == nil || !strings.Contains(err.Error(), "--data") {
		t.Errorf("err = %v", err)
	}
}

func TestLabelsUnknownRun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/key.csv", []byte("id,model,experiment,run_dir,status\nx1,model-a,orders.refund error,model-a/nope/9,pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/labels.csv", []byte("id,verdict\nx1,safe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadAgreement(dir, "testdata/data")
	if err == nil || !strings.Contains(err.Error(), "x1") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v", err)
	}
}

func TestLabelsRejectsKeyWithoutLabel(t *testing.T) {
	_, err := loadAgreement("testdata/labels-missing", "testdata/data")
	if err == nil || !strings.Contains(err.Error(), "r04 (in key, no label)") {
		t.Fatalf("err = %v", err)
	}
	tex := t.TempDir()
	if err := run("testdata/data", "testdata/labels-missing", tex); err == nil {
		t.Error("run: want error")
	}
	if fileExists(tex + "/rq3.tex") {
		t.Error("rq3.tex written")
	}
}
