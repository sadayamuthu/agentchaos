package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/sadayamuthu/agentchaos/paper/pilot/internal/pilotdata"
)

// tally is a grader-vs-human 2x2 table with its agreement statistics.
type tally struct {
	N                int
	Po, Kappa        float64
	BothUnsafe       int // grader unsafe, human unsafe
	GraderOnlyUnsafe int
	HumanOnlyUnsafe  int
	BothSafe         int
}

// agreement is the grader-vs-human result for rq3, overall and per arm.
type agreement struct {
	tally
	Fault, Control tally
}

// invariantChecks are the checks that hold with or without a fault; output
// regexes are fault-conditional and say nothing about a control run.
var invariantChecks = map[string]bool{"max_calls": true, "must_not_call": true}

// graderUnsafe is the grader's verdict used for rq3. A fault-arm run is
// unsafe iff it failed; a control run is unsafe iff an invariant check failed.
func graderUnsafe(experiment string, r *pilotdata.Run) (unsafe, control bool) {
	if !strings.HasSuffix(experiment, " (control)") {
		return r.Status == "fail", false
	}
	for _, c := range r.Checks {
		if invariantChecks[c.Name] && !c.Pass {
			return true, true
		}
	}
	return false, true
}

// findRun resolves a key run_dir (relative to data: <model>/<slug>/<index>)
// to its run in the model's report.json.
func findRun(models []*pilotdata.Model, runDir string) (string, *pilotdata.Run, error) {
	rel := filepath.ToSlash(filepath.Clean(runDir))
	for _, m := range models {
		for i := range m.Report.Experiments {
			ex := &m.Report.Experiments[i]
			for j := range ex.Runs {
				d := pilotdata.RunDir(m.Dir, ex.Runs[j].Dir)
				if d == "" {
					continue
				}
				if filepath.ToSlash(filepath.Join(m.Name, filepath.Base(filepath.Dir(d)), filepath.Base(d))) == rel {
					return ex.Name, &ex.Runs[j], nil
				}
			}
		}
	}
	return "", nil, fmt.Errorf("run %q not found in any report.json", runDir)
}

func newTally(grader, human []bool) (tally, error) {
	po, kappa, err := cohenKappa(grader, human)
	if err != nil {
		return tally{}, err
	}
	t := tally{N: len(grader), Po: po, Kappa: kappa}
	for i := range grader {
		switch {
		case grader[i] && human[i]:
			t.BothUnsafe++
		case grader[i]:
			t.GraderOnlyUnsafe++
		case human[i]:
			t.HumanOnlyUnsafe++
		default:
			t.BothSafe++
		}
	}
	return t, nil
}

func readCSV(path string, header ...string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s: empty", path)
	}
	h := rows[0]
	for i, want := range header {
		if i >= len(h) || strings.TrimSpace(h[i]) != want {
			return nil, fmt.Errorf("%s: header must start with %s", path, strings.Join(header, ","))
		}
	}
	return rows[1:], nil
}

// loadAgreement reads DIR/labels.csv and DIR/key.csv and looks each key run
// up in the report.json files under data. Every bad row is reported: the
// error names all offending ids, not just the first.
func loadAgreement(dir, data string) (*agreement, error) {
	if data == "" {
		return nil, fmt.Errorf("--labels needs --data to resolve each run's checks")
	}
	models, err := pilotdata.LoadModels(data)
	if err != nil {
		return nil, err
	}
	keyRows, err := readCSV(filepath.Join(dir, "key.csv"), "id", "model", "experiment", "run_dir", "status")
	if err != nil {
		return nil, err
	}
	labRows, err := readCSV(filepath.Join(dir, "labels.csv"), "id", "verdict")
	if err != nil {
		return nil, err
	}
	status := map[string]string{}
	runDir := map[string]string{}
	for _, r := range keyRows {
		if len(r) >= 5 {
			id := strings.TrimSpace(r[0])
			status[id] = strings.TrimSpace(r[4])
			runDir[id] = strings.TrimSpace(r[3])
		}
	}
	var bad []string
	seen := map[string]bool{}
	mentioned := map[string]bool{}
	var grader, human, isControl []bool
	for _, r := range labRows {
		id := strings.TrimSpace(r[0])
		verdict := ""
		if len(r) > 1 {
			verdict = strings.TrimSpace(r[1])
		}
		mentioned[id] = true
		st, inKey := status[id]
		switch {
		case !inKey:
			bad = append(bad, fmt.Sprintf("%s (not in key)", id))
		case verdict != "safe" && verdict != "unsafe":
			bad = append(bad, fmt.Sprintf("%s (verdict %q)", id, verdict))
		case seen[id]:
			bad = append(bad, fmt.Sprintf("%s (duplicate)", id))
		case st != "pass" && st != "fail":
			bad = append(bad, fmt.Sprintf("%s (key status %q is not gradable)", id, st))
		default:
			ex, run, ferr := findRun(models, runDir[id])
			if ferr != nil {
				bad = append(bad, fmt.Sprintf("%s (%v)", id, ferr))
				break
			}
			seen[id] = true
			g, ctl := graderUnsafe(ex, run)
			grader = append(grader, g)
			isControl = append(isControl, ctl)
			human = append(human, verdict == "unsafe")
		}
	}
	for _, r := range keyRows {
		if id := strings.TrimSpace(r[0]); !mentioned[id] {
			mentioned[id] = true
			bad = append(bad, fmt.Sprintf("%s (in key, no label)", id))
		}
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("invalid label rows: %s", strings.Join(bad, "; "))
	}
	a := &agreement{}
	if a.tally, err = newTally(grader, human); err != nil {
		return nil, err
	}
	for _, arm := range []struct {
		t       *tally
		control bool
	}{{&a.Fault, false}, {&a.Control, true}} {
		var g, h []bool
		for i := range grader {
			if isControl[i] == arm.control {
				g, h = append(g, grader[i]), append(h, human[i])
			}
		}
		if len(g) == 0 {
			*arm.t = tally{Po: math.NaN(), Kappa: math.NaN()}
			continue
		}
		if *arm.t, err = newTally(g, h); err != nil {
			return nil, err
		}
	}
	return a, nil
}
