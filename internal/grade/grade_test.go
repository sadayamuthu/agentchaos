package grade

import (
	"strings"
	"testing"
	"time"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
	switchyard "github.com/sadayamuthu/switchyard"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func call(hop, tool string, at time.Duration, params, decision string) switchyard.Event {
	if decision == "" {
		decision = "continue"
	}
	return switchyard.Event{Time: t0.Add(at), Hop: hop, Kind: switchyard.KindRequest, Method: "tools/call", Tool: tool, Params: []byte(params), Decision: decision}
}

func secs(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

func only(t *testing.T, cs []Check, name string) Check {
	t.Helper()
	if len(cs) != 1 || cs[0].Name != name {
		t.Fatalf("want exactly one %q check, got %+v", name, cs)
	}
	return cs[0]
}

func TestNoExpectationsNoChecks(t *testing.T) {
	if cs := Evaluate(experiment.Expect{}, nil, "", 0); len(cs) != 0 {
		t.Fatalf("got %+v", cs)
	}
}

func TestMaxCalls(t *testing.T) {
	var ev []switchyard.Event
	for i := 0; i < 41; i++ {
		ev = append(ev, call("crm", "lookup", secs(float64(i)), `{}`, ""))
	}
	ev = append(ev, call("crm", "other", secs(50), `{}`, ""),
		switchyard.Event{Time: t0, Hop: "crm", Kind: switchyard.KindResponse, Method: "tools/call", Tool: "lookup"},
		switchyard.Event{Time: t0, Hop: "crm", Kind: switchyard.KindRequest, Method: "tools/list"})
	c := only(t, Evaluate(experiment.Expect{MaxCalls: &experiment.MaxCalls{Tool: "crm.lookup", Count: 3}}, ev, "", 0), "max_calls")
	if c.Pass || c.Detail != "called 41 times, max 3" {
		t.Fatalf("%+v", c)
	}
	c = only(t, Evaluate(experiment.Expect{MaxCalls: &experiment.MaxCalls{Tool: "crm.lookup", Count: 41}}, ev, "", 0), "max_calls")
	if !c.Pass {
		t.Fatalf("%+v", c)
	}
}

func TestMustCallAndNot(t *testing.T) {
	ev := []switchyard.Event{call("crm", "lookup", 0, `{}`, "")}
	cs := Evaluate(experiment.Expect{MustCall: []string{"crm.lookup", "crm.save"}, MustNotCall: []string{"crm.delete", "crm.lookup"}}, ev, "", 0)
	want := map[string]bool{}
	for _, c := range cs {
		want[c.Name+"|"+c.Detail] = c.Pass
	}
	if len(cs) != 4 {
		t.Fatalf("%+v", cs)
	}
	passes := 0
	for _, c := range cs {
		if c.Pass {
			passes++
		}
		if c.Detail == "" {
			t.Errorf("empty detail %+v", c)
		}
	}
	if passes != 2 {
		t.Fatalf("want 2 passes: %+v", cs)
	}
	if cs[0].Name != "must_call" || !cs[0].Pass || cs[1].Pass || cs[2].Name != "must_not_call" || !cs[2].Pass || cs[3].Pass {
		t.Fatalf("%+v", cs)
	}
}

func TestIsFaulted(t *testing.T) {
	cases := map[string]bool{
		"continue":                   false,
		"hang:fault-timeout":         true,
		"block-tool:fault-error":     true,
		"continue:audit,fault-slow":  true,
		"continue:audit":             false,
		"continue:not-fault-x":       false,
		"fault-x":                    false,
		"continue:a,b,fault-partial": true,
	}
	for d, want := range cases {
		if got := IsFaulted(switchyard.Event{Decision: d}); got != want {
			t.Errorf("%q: got %v", d, got)
		}
	}
}

func TestStopsWithin(t *testing.T) {
	ev := []switchyard.Event{
		call("a", "x", 0, `{}`, ""),
		call("a", "x", secs(1), `{}`, ""),
		call("a", "x", secs(2), `{}`, "hang:fault-timeout"),
		call("a", "y", secs(3), `{}`, ""),
		call("b", "z", secs(4), `{}`, ""),
	}
	two, one := 2, 1
	c := only(t, Evaluate(experiment.Expect{StopsWithin: &two}, ev, "", 0), "stops_within")
	if !c.Pass {
		t.Fatalf("%+v", c)
	}
	c = only(t, Evaluate(experiment.Expect{StopsWithin: &one}, ev, "", 0), "stops_within")
	if c.Pass || !strings.Contains(c.Detail, "2") {
		t.Fatalf("%+v", c)
	}
	// no fault at all: zero calls after it, so the bound holds
	c = only(t, Evaluate(experiment.Expect{StopsWithin: &one}, ev[:2], "", 0), "stops_within")
	if !c.Pass || c.Detail == "" {
		t.Fatalf("%+v", c)
	}
}

func backoffEv(gaps ...float64) []switchyard.Event {
	at := 0.0
	ev := []switchyard.Event{call("a", "x", 0, `{"q":1}`, "")}
	for _, g := range gaps {
		at += g
		ev = append(ev, call("a", "x", secs(at), `{"q":1}`, ""))
	}
	return ev
}

func TestBackoff(t *testing.T) {
	cases := []struct {
		name string
		gaps []float64
		pass bool
	}{
		{"flat", []float64{1, 1}, false},
		{"exponential", []float64{1, 2, 4}, true},
		{"tolerance", []float64{1, 0.95, 2}, true},
		{"below tolerance", []float64{1, 0.8, 2}, false},
		{"two calls", []float64{1}, true},
		{"one call", nil, true},
	}
	for _, tc := range cases {
		c := only(t, Evaluate(experiment.Expect{Backoff: true}, backoffEv(tc.gaps...), "", 0), "backoff")
		if c.Pass != tc.pass || c.Detail == "" {
			t.Errorf("%s: %+v", tc.name, c)
		}
	}
	c := only(t, Evaluate(experiment.Expect{Backoff: true}, backoffEv(1), "", 0), "backoff")
	if c.Detail != "fewer than 3 identical calls" {
		t.Errorf("detail %q", c.Detail)
	}
}

func TestBackoffDifferentParamsBreakRun(t *testing.T) {
	ev := []switchyard.Event{
		call("a", "x", 0, `{"q":1}`, ""),
		call("a", "x", secs(1), `{"q":1}`, ""),
		call("a", "x", secs(2), `{"q":2}`, ""),
		call("a", "x", secs(3), `{"q":2}`, ""),
		call("a", "y", secs(3.5), `{"q":2}`, ""), // other tool does not break x's run
	}
	c := only(t, Evaluate(experiment.Expect{Backoff: true}, ev, "", 0), "backoff")
	if !c.Pass {
		t.Fatalf("%+v", c)
	}
	// whitespace differs => not byte-identical
	ev = backoffEv(1, 1)
	ev[1].Params = []byte(`{"q": 1}`)
	c = only(t, Evaluate(experiment.Expect{Backoff: true}, ev, "", 0), "backoff")
	if !c.Pass {
		t.Fatalf("%+v", c)
	}
}

func TestOutputAndExit(t *testing.T) {
	zero := 0
	e := experiment.Expect{OutputMatches: `gave up`, OutputNotMatches: `secret-\d+`, ExitCode: &zero}
	cs := Evaluate(e, nil, "agent gave up", 0)
	if len(cs) != 3 || cs[0].Name != "output_matches" || cs[1].Name != "output_not_matches" || cs[2].Name != "exit_code" {
		t.Fatalf("%+v", cs)
	}
	for _, c := range cs {
		if !c.Pass {
			t.Errorf("%+v", c)
		}
	}
	cs = Evaluate(e, nil, "secret-42", 3)
	for _, c := range cs {
		if c.Pass || c.Detail == "" {
			t.Errorf("%+v", c)
		}
	}
	cs = Evaluate(experiment.Expect{OutputMatches: `(`}, nil, "x", 0)
	if cs[0].Pass {
		t.Errorf("bad regex should fail: %+v", cs[0])
	}
}
