package main

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/sadayamuthu/agentchaos/paper/pilot/internal/pilotdata"
)

// wilson returns the 95% Wilson score interval for k successes in n trials.
func wilson(k, n int) (lo, hi float64) {
	if n <= 0 {
		return 0, 0
	}
	const z = 1.96
	nf := float64(n)
	p := float64(k) / nf
	z2 := z * z
	denom := 1 + z2/nf
	center := (p + z2/(2*nf)) / denom
	half := z * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf)) / denom
	lo, hi = center-half, center+half
	if k == 0 || lo < 0 {
		lo = 0
	}
	if k == n || hi > 1 {
		hi = 1
	}
	return lo, hi
}

// cohenKappa returns the observed agreement and Cohen's kappa for two
// binary raters. Kappa is NaN when chance agreement is 1 (undefined).
func cohenKappa(a, b []bool) (po, kappa float64, err error) {
	if len(a) != len(b) {
		return 0, 0, fmt.Errorf("length mismatch: %d vs %d", len(a), len(b))
	}
	n := float64(len(a))
	if n == 0 {
		return 0, 0, errors.New("no ratings")
	}
	var agree, aTrue, bTrue float64
	for i := range a {
		if a[i] == b[i] {
			agree++
		}
		if a[i] {
			aTrue++
		}
		if b[i] {
			bTrue++
		}
	}
	po = agree / n
	pe := (aTrue/n)*(bTrue/n) + (1-aTrue/n)*(1-bTrue/n)
	if pe == 1 {
		return po, math.NaN(), nil
	}
	return po, (po - pe) / (1 - pe), nil
}

func meanSD(xs []float64) (mean, sd float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	if len(xs) < 2 {
		return mean, 0
	}
	var ss float64
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(ss / float64(len(xs)-1))
}

// cell aggregates all runs of one experiment under one model.
type cell struct {
	Model, Experiment string
	Valid, Pass       int
	Excluded          map[string]int
	Calls             []float64
	Seqs              map[string]struct{}
	FirstFail         map[string]int
	// Runs are the per-run statuses kept for label matching and tests.
}

func (c *cell) DistinctSeqs() int { return len(c.Seqs) }

// loadData reads DATA/<model>/report.json for every model directory and
// returns the cells in model then report order. Problems that do not stop
// the analysis (missing run directories) are written to warn.
func loadData(data string, warn func(string)) ([]*cell, error) {
	models, err := pilotdata.LoadModels(data)
	if err != nil {
		return nil, err
	}
	var cells []*cell
	for _, m := range models {
		for _, ex := range m.Report.Experiments {
			c := &cell{Model: m.Name, Experiment: ex.Name,
				Excluded: map[string]int{}, Seqs: map[string]struct{}{}, FirstFail: map[string]int{}}
			for _, r := range ex.Runs {
				if r.Status != "pass" && r.Status != "fail" {
					c.Excluded[r.Status]++
					continue
				}
				c.Valid++
				c.Calls = append(c.Calls, float64(r.ToolCalls))
				if r.Status == "pass" {
					c.Pass++
				} else {
					first := "(none)"
					for _, ck := range r.Checks {
						if !ck.Pass {
							first = ck.Name
							break
						}
					}
					c.FirstFail[first]++
				}
				dir := pilotdata.RunDir(m.Dir, r.Dir)
				if dir == "" {
					warn(fmt.Sprintf("%s / %s run %d: cannot locate run directory from %q", m.Name, ex.Name, r.Index, r.Dir))
					continue
				}
				seq, err := pilotdata.ToolSequence(dir)
				if err != nil {
					warn(fmt.Sprintf("%s / %s run %d: %v", m.Name, ex.Name, r.Index, err))
					continue
				}
				c.Seqs[strings.Join(seq, " > ")] = struct{}{}
			}
			cells = append(cells, c)
		}
	}
	return cells, nil
}
