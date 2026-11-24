package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

var texEscaper = strings.NewReplacer(`&`, `\&`, `%`, `\%`, `_`, `\_`, `#`, `\#`, `$`, `\$`)

func tex(s string) string { return texEscaper.Replace(s) }

func pct(x float64) string { return fmt.Sprintf("%.0f", x*100) }

// renderRQ1 writes pass rate with Wilson CI, calls, and distinct sequences.
func renderRQ1(cells []*cell) string {
	var b strings.Builder
	b.WriteString("\\begin{table*}[t]\n\\centering\n\\caption{Pass rate (95\\% Wilson interval), tool calls per run (mean and sample SD, $n-1$), and distinct tool-call sequences per model and experiment. Err/Inv counts runs with status error or invalid, which are excluded from all other columns.}\n\\label{tab:rq1}\n")
	b.WriteString("\\begin{tabular}{llrrrrrr}\n\\hline\n")
	b.WriteString("Model & Experiment & Valid & Pass rate (\\%) [CI] & Calls mean & Calls SD & Distinct seqs & Err/Inv \\\\\n\\hline\n")
	for _, c := range cells {
		row := []string{tex(c.Model), tex(c.Experiment), fmt.Sprint(c.Valid)}
		if c.Valid == 0 {
			row = append(row, "n/a", "n/a", "n/a", "n/a")
		} else {
			lo, hi := wilson(c.Pass, c.Valid)
			mean, sd := meanSD(c.Calls)
			sdS := "n/a"
			if c.Valid > 1 {
				sdS = fmt.Sprintf("%.2f", sd)
			}
			row = append(row,
				fmt.Sprintf("%s [%s, %s]", pct(float64(c.Pass)/float64(c.Valid)), pct(lo), pct(hi)),
				fmt.Sprintf("%.2f", mean), sdS, fmt.Sprint(c.DistinctSeqs()))
		}
		row = append(row, fmt.Sprintf("%d/%d", c.Excluded["error"], c.Excluded["invalid"]))
		b.WriteString(strings.Join(row, " & ") + " \\\\\n")
	}
	b.WriteString("\\hline\n\\end{tabular}\n\\end{table*}\n")
	return b.String()
}

// renderRQ2 writes the first-failing-check histogram, fault vs. control.
// It returns "" when no fault experiment has a control.
func renderRQ2(cells []*cell) string {
	type key struct{ model, exp string }
	idx := map[key]*cell{}
	for _, c := range cells {
		idx[key{c.Model, c.Experiment}] = c
	}
	type pair struct{ fault, control *cell }
	var pairs []pair
	checks := map[string]bool{}
	for _, c := range cells {
		if strings.HasSuffix(c.Experiment, " (control)") {
			continue
		}
		ctl := idx[key{c.Model, c.Experiment + " (control)"}]
		if ctl == nil {
			continue
		}
		pairs = append(pairs, pair{c, ctl})
		for _, x := range []*cell{c, ctl} {
			for name := range x.FirstFail {
				checks[name] = true
			}
		}
	}
	if len(pairs) == 0 {
		return ""
	}
	var names []string
	for n := range checks {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("\\begin{table}[t]\n\\centering\n\\caption{First failing check among failed runs, fault experiment vs. its control. Controls inject no fault.}\n\\label{tab:rq2}\n")
	b.WriteString("\\begin{tabular}{lll" + strings.Repeat("r", len(names)+2) + "}\n\\hline\n")
	hdr := []string{"Model", "Experiment", "Arm", "Valid", "Fail"}
	for _, n := range names {
		hdr = append(hdr, tex(n))
	}
	b.WriteString(strings.Join(hdr, " & ") + " \\\\\n\\hline\n")
	for _, p := range pairs {
		for _, arm := range []struct {
			label string
			c     *cell
		}{{"fault", p.fault}, {"control", p.control}} {
			row := []string{tex(arm.c.Model), tex(p.fault.Experiment), arm.label,
				fmt.Sprint(arm.c.Valid), fmt.Sprint(arm.c.Valid - arm.c.Pass)}
			for _, n := range names {
				row = append(row, fmt.Sprint(arm.c.FirstFail[n]))
			}
			b.WriteString(strings.Join(row, " & ") + " \\\\\n")
		}
		b.WriteString("\\hline\n")
	}
	b.WriteString("\\end{tabular}\n\\end{table}\n")
	return b.String()
}

func kappaText(k float64) string {
	if math.IsNaN(k) {
		return "n/a"
	}
	return fmt.Sprintf("%.3f", k)
}

// renderRQ3 writes the grader-vs-human 2x2 tables with agreement, overall
// and per arm. The grader's verdict is fail for a fault-arm run and an
// invariant-check failure (max_calls, must_not_call) for a control run.
func renderRQ3(a *agreement) string {
	var b strings.Builder
	b.WriteString("\\begin{table}[t]\n\\centering\n\\caption{Grader vs. human verdicts: ")
	fmt.Fprintf(&b, "all %d runs, raw agreement %.3f, Cohen's $\\kappa$ = %s. Fault arm: $n$=%d, agreement %s, $\\kappa$ = %s. Control arm: $n$=%d, agreement %s, $\\kappa$ = %s. Each cell is grader-unsafe/grader-safe counts as shown.}\n\\label{tab:rq3}\n",
		a.N, a.Po, kappaText(a.Kappa), a.Fault.N, poText(a.Fault), kappaText(a.Fault.Kappa), a.Control.N, poText(a.Control), kappaText(a.Control.Kappa))
	b.WriteString("\\begin{tabular}{llrr}\n\\hline\n")
	b.WriteString("Arm & Grader & Human unsafe & Human safe \\\\\n\\hline\n")
	for _, row := range []struct {
		name string
		t    tally
	}{{"All", a.tally}, {"Fault", a.Fault}, {"Control", a.Control}} {
		fmt.Fprintf(&b, "%s & unsafe & %d & %d \\\\\n", row.name, row.t.BothUnsafe, row.t.GraderOnlyUnsafe)
		fmt.Fprintf(&b, " & safe & %d & %d \\\\\n", row.t.HumanOnlyUnsafe, row.t.BothSafe)
		b.WriteString("\\hline\n")
	}
	b.WriteString("\\end{tabular}\n\\end{table}\n")
	return b.String()
}

func poText(t tally) string {
	if t.N == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.3f", t.Po)
}

// summary is the plain-text stdout report.
func summary(cells []*cell, a *agreement) string {
	var b strings.Builder
	for _, c := range cells {
		fmt.Fprintf(&b, "%s | %s: valid=%d pass=%d", c.Model, c.Experiment, c.Valid, c.Pass)
		if c.Valid > 0 {
			lo, hi := wilson(c.Pass, c.Valid)
			mean, sd := meanSD(c.Calls)
			fmt.Fprintf(&b, " rate=%.2f [%.2f, %.2f] calls=%.2f sd=%.2f seqs=%d", float64(c.Pass)/float64(c.Valid), lo, hi, mean, sd, c.DistinctSeqs())
		}
		if len(c.Excluded) > 0 {
			var ks []string
			for k, v := range c.Excluded {
				ks = append(ks, fmt.Sprintf("%s=%d", k, v))
			}
			sort.Strings(ks)
			fmt.Fprintf(&b, " excluded: %s", strings.Join(ks, ","))
		}
		b.WriteString("\n")
	}
	if a != nil {
		for _, row := range []struct {
			name string
			t    tally
		}{{"all", a.tally}, {"fault", a.Fault}, {"control", a.Control}} {
			fmt.Fprintf(&b, "agreement %s: n=%d po=%s kappa=%s\n", row.name, row.t.N, poText(row.t), kappaText(row.t.Kappa))
		}
	}
	return b.String()
}
