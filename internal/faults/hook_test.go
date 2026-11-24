package faults

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sadayamuthu/switchyard"
	"github.com/sadayamuthu/switchyard/testkit"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
)

const lookupText = `{"plan":"enterprise","seats":40}`

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func testServer(t *testing.T) *mcp.Server {
	t.Helper()
	return testkit.Server(t,
		testkit.ToolSpec{Name: "lookup", Description: "look up an account", Handler: func(context.Context, map[string]any) (*mcp.CallToolResult, error) {
			return textResult(lookupText), nil
		}},
		testkit.ToolSpec{Name: "other", Description: "another tool", Handler: func(context.Context, map[string]any) (*mcp.CallToolResult, error) {
			return textResult("other-ok"), nil
		}},
	)
}

func connect(t *testing.T, f experiment.Fault, sink switchyard.EventSink, extra ...switchyard.Hook) *mcp.ClientSession {
	t.Helper()
	hooks := append(extra, New(f))
	return testkit.Connect(t, testServer(t), switchyard.Options{Name: "test", Hooks: hooks, Sink: sink})
}

func call(ctx context.Context, cs *mcp.ClientSession, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	return cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
}

func firstText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if len(r.Content) == 0 {
		t.Fatalf("result has no content")
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", r.Content[0])
	}
	return tc.Text
}

func TestTimeoutHangs(t *testing.T) {
	cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "timeout", OnCall: 1}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := call(ctx, cs, "lookup", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first call err = %v, want deadline exceeded", err)
	}

	r, err := call(context.Background(), cs, "lookup", nil)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := firstText(t, r); got != lookupText {
		t.Fatalf("second call text = %q, want %q", got, lookupText)
	}
}

func TestSlowDelays(t *testing.T) {
	cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "slow", EveryCall: true, Delay: 300 * time.Millisecond}, nil)
	start := time.Now()
	r, err := call(context.Background(), cs, "lookup", nil)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 300*time.Millisecond {
		t.Fatalf("call took %v, want >= 300ms", d)
	}
	if got := firstText(t, r); got != lookupText {
		t.Fatalf("text = %q, want %q", got, lookupText)
	}
}

func TestErrorToolMode(t *testing.T) {
	cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "error", OnCall: 1, ErrorMode: "tool", Message: "service unavailable"}, nil)
	r, err := call(context.Background(), cs, "lookup", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError {
		t.Fatalf("IsError = false, want true")
	}
	if got := firstText(t, r); got != "service unavailable" {
		t.Fatalf("text = %q", got)
	}
}

func TestErrorJSONRPCMode(t *testing.T) {
	cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "error", OnCall: 1, ErrorMode: "jsonrpc", Message: "internal boom"}, nil)
	_, err := call(context.Background(), cs, "lookup", nil)
	if err == nil {
		t.Fatal("err = nil, want JSON-RPC error")
	}
	if !strings.Contains(err.Error(), "internal boom") {
		t.Fatalf("err = %v, want it to contain the message", err)
	}
}

func TestPartialFraction(t *testing.T) {
	cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "partial", OnCall: 1, Fraction: 0.5}, nil)
	r, err := call(context.Background(), cs, "lookup", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := firstText(t, r), lookupText[:len(lookupText)/2]; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	// Second call is untouched.
	r, err = call(context.Background(), cs, "lookup", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := firstText(t, r); got != lookupText {
		t.Fatalf("second text = %q, want %q", got, lookupText)
	}
}

func TestMalformedInvalidJSON(t *testing.T) {
	cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "malformed", OnCall: 1, Corrupt: "invalid-json"}, nil)
	r, err := call(context.Background(), cs, "lookup", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := firstText(t, r)
	if json.Valid([]byte(got)) {
		t.Fatalf("text %q is valid JSON, want invalid", got)
	}
	if want := lookupText[:len(lookupText)/2] + `{"`; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}

func TestOtherToolUntouched(t *testing.T) {
	rec := &testkit.Recorder{}
	cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "error", EveryCall: true, ErrorMode: "tool", Message: "x"}, rec)
	for range 3 {
		r, err := call(context.Background(), cs, "other", nil)
		if err != nil {
			t.Fatal(err)
		}
		if r.IsError || firstText(t, r) != "other-ok" {
			t.Fatalf("other faulted: %+v", r)
		}
	}
	for _, e := range rec.Events() {
		if e.Kind == switchyard.KindRequest && e.Decision != "continue" {
			t.Fatalf("event %s %s decision = %q, want continue", e.Method, e.Tool, e.Decision)
		}
	}
}

func faultedIndexes(t *testing.T, seed int64) []int {
	t.Helper()
	cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "error", Probability: 0.5, Seed: seed, ErrorMode: "tool", Message: "x"}, nil)
	var idx []int
	for i := 1; i <= 20; i++ {
		r, err := call(context.Background(), cs, "lookup", nil)
		if err != nil {
			t.Fatal(err)
		}
		if r.IsError {
			idx = append(idx, i)
		}
	}
	return idx
}

func TestProbabilitySeedReproducible(t *testing.T) {
	a := faultedIndexes(t, 42)
	b := faultedIndexes(t, 42)
	if !slices.Equal(a, b) {
		t.Fatalf("runs differ: %v vs %v", a, b)
	}
	if len(a) == 0 || len(a) == 20 {
		t.Fatalf("faulted %v: want some but not all of 20 calls faulted at p=0.5", a)
	}
}

// arrivals is a test hook, placed before the fault hook, that reports each
// lookup call's "i" argument and ToolCall.Seq as the proxy hooks it.
type arrivals struct {
	switchyard.Base
	ch chan [2]uint64
}

func (arrivals) Name() string { return "arrivals" }

func (a arrivals) OnRequest(_ context.Context, _ *switchyard.Session, r *switchyard.Request) switchyard.Decision {
	tc, ok := r.ToolCall()
	if !ok || tc.Name != "lookup" {
		return switchyard.Continue()
	}
	var args struct {
		I uint64 `json:"i"`
	}
	_ = json.Unmarshal(tc.Arguments, &args)
	a.ch <- [2]uint64{args.I, tc.Seq}
	return switchyard.Continue()
}

func TestOnCallParallelDeterministic(t *testing.T) {
	for rep := range 20 {
		t.Run(fmt.Sprintf("rep%d", rep), func(t *testing.T) {
			rec := &testkit.Recorder{}
			arr := arrivals{ch: make(chan [2]uint64, 6)}
			cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "error", OnCall: 3, ErrorMode: "tool", Message: "boom"}, rec, arr)

			var wg sync.WaitGroup
			results := make([]*mcp.CallToolResult, 7)
			errs := make([]error, 7)
			// Send all six without awaiting any: each call is launched only
			// after the proxy has read the previous one, so arrival order
			// (and so ToolCall.Seq) is i.
			for i := 1; i <= 6; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					results[i], errs[i] = call(context.Background(), cs, "lookup", map[string]any{"i": i})
				}()
				select {
				case got := <-arr.ch:
					if got[0] != uint64(i) || got[1] != uint64(i) {
						t.Errorf("arrival (i, seq) = %v, want (%d, %d)", got, i, i)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("call %d never reached the proxy", i)
				}
			}
			wg.Wait()

			for i := 1; i <= 6; i++ {
				if errs[i] != nil {
					t.Fatalf("call %d: %v", i, errs[i])
				}
				if want := i == 3; results[i].IsError != want {
					t.Errorf("call %d IsError = %v, want %v", i, results[i].IsError, want)
				}
			}
			var faulted int
			for _, e := range rec.Events() {
				if e.Kind == switchyard.KindRequest && e.Decision == "block-tool:fault-error" {
					faulted++
				}
			}
			if faulted != 1 {
				t.Errorf("faulted request events = %d, want 1", faulted)
			}
		})
	}
}

func TestDecisionAnnotated(t *testing.T) {
	rec := &testkit.Recorder{}
	cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "timeout", OnCall: 1}, rec)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _ = call(ctx, cs, "lookup", nil)

	var found bool
	for _, e := range rec.Events() {
		if e.Kind == switchyard.KindRequest && e.Tool == "lookup" {
			if e.Decision != "hang:fault-timeout" {
				t.Fatalf("decision = %q, want hang:fault-timeout", e.Decision)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no request event for lookup")
	}
}

func TestPassThrough(t *testing.T) {
	h := NewPassThrough()
	if h.Name() != "agentchaos" {
		t.Fatalf("Name = %q", h.Name())
	}
	rec := &testkit.Recorder{}
	cs := testkit.Connect(t, testServer(t), switchyard.Options{Name: "test", Hooks: []switchyard.Hook{h}, Sink: rec})
	r, err := call(context.Background(), cs, "lookup", nil)
	if err != nil {
		t.Fatal(err)
	}
	if firstText(t, r) != lookupText {
		t.Fatalf("text = %q", firstText(t, r))
	}
	for _, e := range rec.Events() {
		if e.Kind == switchyard.KindRequest && e.Decision != "continue" {
			t.Fatalf("decision = %q, want continue", e.Decision)
		}
	}
}

func TestName(t *testing.T) {
	if n := New(experiment.Fault{Tool: "s.lookup", Type: "timeout", OnCall: 1}).Name(); n != "agentchaos" {
		t.Fatalf("Name = %q", n)
	}
}

func toolResult(t *testing.T, raw string) *switchyard.ToolResult {
	t.Helper()
	tr, ok := (&switchyard.Response{Result: json.RawMessage(raw)}).ToolResult()
	if !ok {
		t.Fatalf("not a tool result: %s", raw)
	}
	return tr
}

func TestMutate(t *testing.T) {
	const in = `{"content":[{"type":"text","text":"héllo wörld"},{"type":"image","data":"AAAA","mimeType":"image/png"}],"structuredContent":{"a":1}}`
	tests := []struct {
		name string
		f    experiment.Fault
		want string
	}{
		{"partial bytes on utf8 boundary", experiment.Fault{Type: "partial", Bytes: 2},
			`{"content":[{"text":"h","type":"text"},{"type":"image","data":"AAAA","mimeType":"image/png"}]}`},
		{"partial bytes larger than text", experiment.Fault{Type: "partial", Bytes: 100},
			`{"content":[{"text":"héllo wörld","type":"text"},{"type":"image","data":"AAAA","mimeType":"image/png"}]}`},
		{"partial fraction", experiment.Fault{Type: "partial", Fraction: 0.5},
			`{"content":[{"text":"héllo","type":"text"},{"type":"image","data":"AAAA","mimeType":"image/png"}]}`},
		{"invalid-json", experiment.Fault{Type: "malformed", Corrupt: "invalid-json"},
			`{"content":[{"text":"héllo{\"","type":"text"},{"type":"image","data":"AAAA","mimeType":"image/png"}]}`},
		{"drop-fields", experiment.Fault{Type: "malformed", Corrupt: "drop-fields"},
			`{}`},
		{"wrong-types", experiment.Fault{Type: "malformed", Corrupt: "wrong-types"},
			`{"content":"content"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mutate(tt.f, toolResult(t, in))
			if string(got) != tt.want {
				t.Fatalf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestMutateKeepsIsError(t *testing.T) {
	tr := toolResult(t, `{"content":[{"type":"text","text":"x"}],"isError":true}`)
	if got, want := string(mutate(experiment.Fault{Type: "malformed", Corrupt: "drop-fields"}, tr)), `{"isError":true}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// TestPartialPassesThroughErrors: the upstream does not offer lookup, so it
// answers with a JSON-RPC error, which a recorded partial fault must leave
// unchanged.
func TestPartialPassesThroughErrors(t *testing.T) {
	h := New(experiment.Fault{Tool: "s.lookup", Type: "partial", OnCall: 1, Fraction: 0.5})
	srv := testkit.Server(t, testkit.ToolSpec{Name: "other", Handler: func(context.Context, map[string]any) (*mcp.CallToolResult, error) {
		return textResult("other-ok"), nil
	}})
	direct, derr := call(context.Background(), testkit.ConnectDirect(t, srv), "lookup", nil)
	if derr == nil {
		t.Fatalf("direct call succeeded: %+v", direct)
	}
	cs := testkit.Connect(t, srv, switchyard.Options{Name: "test", Hooks: []switchyard.Hook{h}})
	_, err := call(context.Background(), cs, "lookup", nil)
	if err == nil || err.Error() != derr.Error() {
		t.Fatalf("err = %v, want %v", err, derr)
	}
}

// TestStructuredContentDropped: a server built with typed tools returns the
// same data in structuredContent, so partial and malformed faults must drop
// it or the agent still receives complete, valid data.
func TestStructuredContentDropped(t *testing.T) {
	srv := testkit.Server(t, testkit.ToolSpec{Name: "lookup", Handler: func(context.Context, map[string]any) (*mcp.CallToolResult, error) {
		r := textResult(lookupText)
		r.StructuredContent = map[string]any{"plan": "enterprise", "seats": 40}
		return r, nil
	}})
	direct, err := call(context.Background(), testkit.ConnectDirect(t, srv), "lookup", nil)
	if err != nil || direct.StructuredContent == nil {
		t.Fatalf("direct call: err=%v structured=%v, want structuredContent present", err, direct)
	}
	for _, f := range []experiment.Fault{
		{Tool: "s.lookup", Type: "partial", OnCall: 1, Fraction: 0.5},
		{Tool: "s.lookup", Type: "malformed", OnCall: 1, Corrupt: "invalid-json"},
		{Tool: "s.lookup", Type: "malformed", OnCall: 1, Corrupt: "drop-fields"},
	} {
		t.Run(f.Type+"/"+f.Corrupt, func(t *testing.T) {
			cs := testkit.Connect(t, srv, switchyard.Options{Name: "test", Hooks: []switchyard.Hook{New(f)}})
			r, err := call(context.Background(), cs, "lookup", nil)
			if err != nil {
				t.Fatal(err)
			}
			if r.StructuredContent != nil {
				t.Fatalf("StructuredContent = %v, want dropped", r.StructuredContent)
			}
		})
	}
}

// reorder is a test hook, placed before the fault hook, that holds the
// request with Seq holdSeq until release is closed, so that request reaches
// the hooks after later ones. It reports each lookup arrival's Seq on
// arrived, and appends each Seq to order as the request leaves it.
type reorder struct {
	switchyard.Base
	holdSeq uint64
	release chan struct{}
	arrived chan uint64
	mu      *sync.Mutex
	order   *[]uint64
}

func (reorder) Name() string { return "reorder" }

func (h reorder) OnRequest(ctx context.Context, _ *switchyard.Session, r *switchyard.Request) switchyard.Decision {
	tc, ok := r.ToolCall()
	if !ok || tc.Name != "lookup" {
		return switchyard.Continue()
	}
	h.arrived <- tc.Seq
	if tc.Seq == h.holdSeq {
		select {
		case <-h.release:
		case <-ctx.Done():
		}
	}
	h.mu.Lock()
	*h.order = append(*h.order, tc.Seq)
	h.mu.Unlock()
	return switchyard.Continue()
}

// TestOnCallTargetsSeqOutOfOrder: the Seq-3 request reaches the fault hook
// fourth, yet on_call: 3 faults exactly that call, so the hook targets by
// ToolCall.Seq and not by a counter.
func TestOnCallTargetsSeqOutOfOrder(t *testing.T) {
	for rep := range 5 {
		t.Run(fmt.Sprintf("rep%d", rep), func(t *testing.T) {
			var mu sync.Mutex
			var order []uint64
			ro := reorder{holdSeq: 3, release: make(chan struct{}), arrived: make(chan uint64, 4), mu: &mu, order: &order}
			rec := &testkit.Recorder{}
			cs := connect(t, experiment.Fault{Tool: "s.lookup", Type: "error", OnCall: 3, ErrorMode: "tool", Message: "boom"}, rec, ro)

			var wg sync.WaitGroup
			results := make([]*mcp.CallToolResult, 5)
			errs := make([]error, 5)
			launch := func(i int) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					results[i], errs[i] = call(context.Background(), cs, "lookup", nil)
				}()
				select {
				case got := <-ro.arrived:
					if got != uint64(i) {
						t.Errorf("arrival Seq = %d, want %d", got, i)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("call %d never reached the proxy", i)
				}
			}
			for i := 1; i <= 4; i++ {
				launch(i)
			}
			// Calls 1, 2 and 4 complete while call 3 is still held back.
			deadline := time.Now().Add(5 * time.Second)
			for {
				mu.Lock()
				n := len(order)
				mu.Unlock()
				if n == 3 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("calls 1, 2, 4 did not pass the reorder hook (passed %d)", n)
				}
				time.Sleep(5 * time.Millisecond)
			}
			close(ro.release)
			wg.Wait()

			mu.Lock()
			got := slices.Clone(order)
			mu.Unlock()
			if want := []uint64{1, 2, 4, 3}; !slices.Equal(got, want) {
				t.Fatalf("hook arrival order = %v, want %v", got, want)
			}
			for i := 1; i <= 4; i++ {
				if errs[i] != nil {
					t.Fatalf("call %d: %v", i, errs[i])
				}
				if want := i == 3; results[i].IsError != want {
					t.Errorf("call %d IsError = %v, want %v", i, results[i].IsError, want)
				}
			}
			var faulted int
			for _, e := range rec.Events() {
				if e.Kind == switchyard.KindRequest && e.Decision == "block-tool:fault-error" {
					faulted++
				}
			}
			if faulted != 1 {
				t.Errorf("faulted request events = %d, want 1", faulted)
			}
		})
	}
}
