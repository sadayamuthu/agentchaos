// Package faults is the switchyard hook that injects one experiment Fault
// into calls to its target tool.
package faults

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/sadayamuthu/switchyard"
	"github.com/sadayamuthu/switchyard/jsonrpc"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
)

// hookName is the name of every hook this package returns.
const hookName = "agentchaos"

// New returns a hook applying f to tools/call requests whose tool name is
// f.ToolName(). Every other request is continued unannotated.
func New(f experiment.Fault) switchyard.Hook {
	return &hook{f: f, tool: f.ToolName()}
}

// NewPassThrough returns a hook named agentchaos that always continues, for
// servers with no fault.
func NewPassThrough() switchyard.Hook { return passThrough{} }

type passThrough struct{ switchyard.Base }

func (passThrough) Name() string { return hookName }

type hook struct {
	f    experiment.Fault
	tool string
}

func (h *hook) Name() string { return hookName }

// triggered reports whether the call with per-tool index seq is faulted.
func (h *hook) triggered(seq uint64) bool {
	switch {
	case h.f.EveryCall:
		return true
	case h.f.OnCall > 0:
		return seq == uint64(h.f.OnCall)
	case h.f.Probability > 0:
		return rand.New(rand.NewPCG(uint64(h.f.Seed), seq)).Float64() < h.f.Probability
	}
	return false
}

// pendingKey is the session key holding the set of Request.Seq values whose
// responses a partial or malformed fault must mutate. The set is only ever
// read or written inside Session.Update, which holds the session lock.
const pendingKey = "agentchaos/pending"

// markPending records seq as awaiting a response mutation.
func markPending(s *switchyard.Session, seq uint64) {
	s.Update(pendingKey, func(old any) any {
		m, _ := old.(map[uint64]struct{})
		if m == nil {
			m = map[uint64]struct{}{}
		}
		m[seq] = struct{}{}
		return m
	})
}

// takePending reports whether seq was recorded, and deletes it.
func takePending(s *switchyard.Session, seq uint64) bool {
	var found bool
	s.Update(pendingKey, func(old any) any {
		m, _ := old.(map[uint64]struct{})
		if _, found = m[seq]; found {
			delete(m, seq)
		}
		return old
	})
	return found
}

func (h *hook) OnRequest(ctx context.Context, s *switchyard.Session, r *switchyard.Request) switchyard.Decision {
	tc, ok := r.ToolCall()
	if !ok || tc.Name != h.tool || !h.triggered(tc.Seq) {
		return switchyard.Continue()
	}
	label := "fault-" + h.f.Type
	switch h.f.Type {
	case "timeout":
		return switchyard.Hang().Annotate(label)
	case "slow":
		t := time.NewTimer(h.f.Delay)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			// The request is going away; the decision is discarded anyway.
		}
		return switchyard.Continue().Annotate(label)
	case "error":
		if h.f.ErrorMode == "jsonrpc" {
			return switchyard.Block(&jsonrpc.Error{Code: -32603, Message: h.f.Message}).Annotate(label)
		}
		return switchyard.BlockTool(h.f.Message).Annotate(label)
	case "partial", "malformed":
		markPending(s, r.Seq)
		return switchyard.Continue().Annotate(label)
	}
	return switchyard.Continue()
}

func (h *hook) OnResponse(_ context.Context, s *switchyard.Session, r *switchyard.Request, resp *switchyard.Response) *switchyard.Response {
	if !takePending(s, r.Seq) {
		return resp
	}
	tr, ok := resp.ToolResult()
	if !ok {
		return resp
	}
	return &switchyard.Response{Result: mutate(h.f, tr)}
}
