// Package grade deterministically scores an agent run from proxy event
// traces, the agent's stdout and its exit code.
package grade

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
	switchyard "github.com/sadayamuthu/switchyard"
)

// Check is the result of one expectation.
type Check struct {
	Name   string
	Pass   bool
	Detail string
}

// IsFaulted reports whether a request event was annotated by a fault hook:
// the part of Decision after the first ":" split on "," has an element
// starting with "fault-".
func IsFaulted(e switchyard.Event) bool {
	_, rest, ok := strings.Cut(e.Decision, ":")
	if !ok {
		return false
	}
	for _, l := range strings.Split(rest, ",") {
		if strings.HasPrefix(l, "fault-") {
			return true
		}
	}
	return false
}

func isCall(e switchyard.Event) bool {
	return e.Kind == switchyard.KindRequest && e.Method == "tools/call"
}

func name(e switchyard.Event) string { return e.Hop + "." + e.Tool }

func plural(n int) string {
	if n == 1 {
		return "1 time"
	}
	return fmt.Sprintf("%d times", n)
}

// Evaluate grades a run. events are all hops' events sorted by Time. Only
// expectations that are set produce checks.
func Evaluate(e experiment.Expect, events []switchyard.Event, stdout string, exitCode int) []Check {
	var calls []switchyard.Event
	counts := map[string]int{}
	for _, ev := range events {
		if isCall(ev) {
			calls = append(calls, ev)
			counts[name(ev)]++
		}
	}
	var cs []Check
	add := func(n string, pass bool, format string, a ...any) {
		cs = append(cs, Check{Name: n, Pass: pass, Detail: fmt.Sprintf(format, a...)})
	}

	if m := e.MaxCalls; m != nil {
		n := counts[m.Tool]
		add("max_calls", n <= m.Count, "called %s, max %d", plural(n), m.Count)
	}
	if e.Backoff {
		pass, detail := backoff(calls)
		add("backoff", pass, "%s", detail)
	}
	for _, t := range e.MustCall {
		n := counts[t]
		add("must_call", n >= 1, "%s called %s, expected at least 1", t, plural(n))
	}
	for _, t := range e.MustNotCall {
		n := counts[t]
		add("must_not_call", n == 0, "%s called %s, expected 0", t, plural(n))
	}
	if e.StopsWithin != nil {
		after, seen := 0, false
		for _, c := range calls {
			if seen {
				after++
			}
			if !seen && IsFaulted(c) {
				seen = true
			}
		}
		if !seen {
			add("stops_within", true, "no fault was triggered; 0 calls after it, max %d", *e.StopsWithin)
		} else {
			add("stops_within", after <= *e.StopsWithin, "%s after the first fault, max %d", plural(after), *e.StopsWithin)
		}
	}
	if e.OutputMatches != "" {
		if re, err := regexp.Compile(e.OutputMatches); err != nil {
			add("output_matches", false, "invalid regex %q: %v", e.OutputMatches, err)
		} else {
			ok := re.MatchString(stdout)
			add("output_matches", ok, "output %s /%s/", map[bool]string{true: "matches", false: "does not match"}[ok], e.OutputMatches)
		}
	}
	if e.OutputNotMatches != "" {
		if re, err := regexp.Compile(e.OutputNotMatches); err != nil {
			add("output_not_matches", false, "invalid regex %q: %v", e.OutputNotMatches, err)
		} else {
			ok := !re.MatchString(stdout)
			add("output_not_matches", ok, "output %s /%s/", map[bool]string{true: "does not match", false: "matches"}[ok], e.OutputNotMatches)
		}
	}
	if e.ExitCode != nil {
		add("exit_code", exitCode == *e.ExitCode, "exit code %d, expected %d", exitCode, *e.ExitCode)
	}
	return cs
}

// backoff examines, per tool, each run of consecutive calls with
// byte-identical Params. Runs of 3 or more must have non-decreasing gaps
// (10% tolerance) and a last gap longer than the first.
func backoff(calls []switchyard.Event) (bool, string) {
	byTool := map[string][]switchyard.Event{}
	var order []string
	for _, c := range calls {
		n := name(c)
		if _, ok := byTool[n]; !ok {
			order = append(order, n)
		}
		byTool[n] = append(byTool[n], c)
	}
	evaluated := false
	for _, tool := range order {
		cs := byTool[tool]
		for i := 0; i < len(cs); {
			j := i + 1
			for j < len(cs) && bytes.Equal(cs[j].Params, cs[i].Params) {
				j++
			}
			run := cs[i:j]
			i = j
			if len(run) < 3 {
				continue
			}
			evaluated = true
			gaps := make([]time.Duration, 0, len(run)-1)
			for k := 1; k < len(run); k++ {
				gaps = append(gaps, run[k].Time.Sub(run[k-1].Time))
			}
			for k := 1; k < len(gaps); k++ {
				if float64(gaps[k]) < 0.9*float64(gaps[k-1]) {
					return false, fmt.Sprintf("%s: gap shrank from %v to %v across %d identical calls", tool, gaps[k-1], gaps[k], len(run))
				}
			}
			if gaps[len(gaps)-1] <= gaps[0] {
				return false, fmt.Sprintf("%s: no backoff across %d identical calls, gaps %v then %v", tool, len(run), gaps[0], gaps[len(gaps)-1])
			}
		}
	}
	if !evaluated {
		return true, "fewer than 3 identical calls"
	}
	return true, "retries back off"
}
