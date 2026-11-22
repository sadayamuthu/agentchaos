# switchyard + AgentChaos v0: design

Date: 2026-11-22
Status: draft for approval

## Intent

Build the shared MCP interception layer for OpenAstra's runtime tools
(**switchyard**) and its first consumer (**AgentChaos**) together, so the
layer is shaped by a real product under test pressure rather than designed in
the abstract.

Why AgentChaos first: the user chose it because it forces the shared proxy to
exist. IntentLock, Taintline, Fuse and Rewind will all be built on switchyard
later. Success for this work is therefore two things: AgentChaos v0 produces a
useful resilience report on a real agent, and switchyard's interface is
demonstrably sufficient for the other four (checked on paper, below).

See `agentchaos/THESIS.md` for the product bet and what would falsify it.

## Decisions made with the user

- **switchyard is its own repo from day one** (`github.com/sadayamuthu/switchyard`),
  not a module inside AgentChaos. AgentChaos depends on it like any other
  consumer.
- **Go**, for switchyard, AgentChaos and every new OpenAstra tool.
- **Composition by chaining.** switchyard is a library; each tool is its own
  binary acting as an MCP proxy. Tools stack by pointing one at the next:
  `agent -> fuse -> intentlock -> taintline -> rewind -> MCP server`. No
  combined plugin binary in v0 (may be revisited if per-hop latency matters).
- Name `switchyard` was proposed by Claude; user did not object. Rename is
  cheap before first release.

## Goals

switchyard v0:
- A transparent stdio MCP proxy library: one side is an MCP server to the
  downstream client (agent or next proxy in the chain), the other spawns and
  talks to the upstream MCP server.
- A hook interface that can observe, delay, block, synthesize and rewrite
  `tools/call` traffic.
- Per-session state that hooks can use across calls.
- A structured event trace (JSONL) of every call, decision and result.
- A test kit (scriptable fake MCP server and fake client) that every consumer
  reuses.

AgentChaos v0:
- `agentchaos proxy`: switchyard + fault-injection hook, driven by an
  experiment file.
- `agentchaos run`: runs an agent against a task with the proxy in place,
  repeats N times, grades behaviour from the trace, prints a report.
- Faults: `timeout`, `slow`, `error`, `partial`, `malformed`.
- Deterministic grading from trace + agent exit code + simple output checks.

## Non-goals (v0)

- Streamable HTTP transport (stdio only; HTTP is the first follow-up).
- `poisoned`, `stale` and `rogue-peer` faults.
- Model-judged grading (e.g. "fabricated a value").
- GitHub Action, HTML report, score trends.
- Airlock integration; experiments are expected to run against staging or
  harmless servers.
- In-process Python/TypeScript framework wrappers (conflicts with Go-only;
  to be decided when Taintline/Rewind need them).

## Anti-goals

- **switchyard must not contain tool-specific logic.** No fault injection,
  policy, or counting in the library; those live in consumers. If a feature
  only makes sense for one tool, it doesn't go in switchyard.
- **The proxy must be transparent by default.** With no hooks registered,
  a client must not be able to tell switchyard is there (same initialize
  result, capabilities, tool list, notifications, errors), so chains of
  tools don't interfere with each other.
- **No model in the loop** in switchyard or in v0 grading.

## Constraints

- Go; release as single static binaries via the existing Homebrew tap
  (`sadayamuthu/homebrew-tap`), like mergelock.
- Must work on macOS and Linux.
- Proxy overhead per hop should be small enough that chaining four tools is
  not noticeable to a person (target: low single-digit milliseconds per call
  per hop; measure, don't assume).

## Approaches considered

Where the shared proxy lives:
1. **Own repo from day one (chosen).** Clean boundary and independent
   releases; risk is designing an interface before the second consumer
   exists. Mitigated by the consumer check below.
2. Module inside AgentChaos, extracted later. Faster start, interface shaped
   by two real users, but extraction work later.
3. Build on an existing MCP gateway. Fast start, but the interception model
   that every tool depends on would belong to someone else.

How tools combine:
1. **Separate binaries, chained (chosen).** Independent repos and releases;
   chain order is the order of checks; costs a hop per tool and one config
   each.
2. One binary with all tools as plugins. Fastest and one config, but couples
   every release.
3. Chaining now, optional bundle later. Kept as the fallback if latency bites.

## switchyard design

### Message handling (Claude's call)

Operate on **JSON-RPC messages**, not on a full MCP SDK server/client pair.
switchyard frames and forwards every message in both directions untouched,
and only decodes the ones it intercepts (`tools/call` requests and their
responses; `tools/list` responses are exposed read-only for consumers that
need tool schemas). Rationale: transparency for methods and capabilities
switchyard doesn't know about (sampling, elicitation, roots, future spec
additions) without re-implementing them. The official Go MCP SDK may be used
for types and framing helpers if it fits; the builder decides.

Request IDs: switchyard must handle a hook synthesizing a response (never
forwarding upstream) and must keep ID mapping correct when it does.
Cancellation (`notifications/cancelled`) and timeouts must not leak pending
entries.

### Hook interface (sketch; exact names to the builder)

```go
type Hook interface {
    // Before forwarding upstream.
    OnCall(ctx context.Context, s *Session, c *Call) Decision
    // After upstream responds (not called if OnCall synthesized/blocked).
    OnResult(ctx context.Context, s *Session, c *Call, r *Result) *Result
}

// Decision is one of:
//   Continue            forward as-is (possibly after the hook slept: delay)
//   ContinueWith(args)  forward with rewritten arguments
//   Block(err)          return a JSON-RPC error or tool isError result
//   Respond(result)     return a synthesized result, never call upstream
//   Hang                never respond (timeout fault); cleaned up on cancel/close
```

- A proxy has an ordered list of hooks; first non-`Continue` decision wins.
- `Hold` for human approval (Rewind, Fuse reset) is **not** in v0, but the
  `Decision` type must be able to grow it without breaking consumers.
- **Hooks are keyed by MCP method, not hard-wired to `tools/call`** (Claude's
  call, after review). The core interception point is a generic
  request/response pair by method; `tools/call` gets typed convenience
  wrappers. v0 implements decisions for `tools/call`, allows rewriting
  `tools/list` results (IntentLock may hide tools), and lets hooks observe
  `resources/read` and `prompts/get` results (Taintline sources). Adding
  methods later must not change the hook signature.
- `Session` = one downstream connection. Provides a concurrency-safe key/value
  store, session ID, and the upstream tool list.
- Event sink interface; default writes JSONL. Each event: timestamp, session
  ID, hop name, call ID, tool, args, decision, result summary, latency.
  Schema is versioned from the first release.
- **Cross-hop correlation** (Claude's call, after review): each hop spawns
  the next as a child process, so a chain ID travels in the environment
  (`SWITCHYARD_CHAIN_ID`; the first hop generates it if absent). Every event
  carries it, so traces from all hops in a chain can be joined. Nothing is
  added to MCP messages, preserving transparency.

### Consumer check (must hold before v0 ships)

| Consumer | Needs | Covered by |
|----------|-------|-----------|
| AgentChaos | delay, fail, hang, rewrite/truncate results | OnCall (sleep, Block, Hang, Respond), OnResult |
| IntentLock | block by tool + args, before execution | OnCall Block; Session for envelope |
| Taintline | see tool outputs as untrusted sources, match later args against them, block | OnResult (record into Session), OnCall Block |
| Fuse | count per agent/session, block when tripped, emit event | OnCall + Session counters, Block, event sink |
| Rewind | record call + result, capture undo data | OnCall/OnResult + event sink; Hold later |

Known gap, accepted: none of the four can see the agent's full context, only
what crosses the tool boundary. Taintline's "label the context" is limited to
tool outputs in proxy mode; that is consistent with its README's proxy mode.

### Packaging

- Go module `github.com/sadayamuthu/switchyard`, library only plus a
  `switchyard` debug binary (pass-through proxy that just writes the trace;
  useful for users and for testing chains).
- `testkit` package: scriptable fake MCP server (declare tools and canned or
  scripted responses) and fake client. Used by switchyard's own tests and by
  every consumer.

## AgentChaos v0 design

### Commands

```console
agentchaos proxy --experiment exp.yaml -- <upstream server command...>
agentchaos run experiments/support-agent.yaml
```

`proxy` is the building block (also usable by hand inside any MCP config).
`run` orchestrates.

### Running an agent (Claude's call)

An **agent adapter is a command template** with placeholders for the task
prompt and a generated MCP config file, e.g.
`claude -p {prompt} --mcp-config {mcp_config} --strict-mcp-config`.
`run` generates the MCP config so that each upstream server is wrapped by
`agentchaos proxy`, runs the command, captures stdout and exit code, and
collects the proxy trace. Claude Code headless ships as a built-in preset;
any other agent works through a custom template. This avoids writing a
reference agent in v0.

### Experiment file (sketch)

```yaml
agent:
  preset: claude-code          # or: command: "my-agent --prompt {prompt} --mcp {mcp_config}"
servers:
  crm: { command: ["./crm-server"] }
  orders: { command: ["./orders-server"] }
runs: 5                         # repeat each experiment; report pass rate
experiments:
  - name: crm.lookup timeout
    task: "What plan is customer 4411 on?"
    fault: { tool: crm.lookup, type: timeout, on_call: 1 }
    expect:
      max_calls: { tool: crm.lookup, count: 4 }
      backoff: true
      output_matches: "(unavailable|couldn't|unable)"
  - name: orders.refund error
    task: "Refund order 4411"
    fault: { tool: orders.refund, type: error, every_call: true }
    expect:
      max_calls: { tool: orders.refund, count: 3 }
      must_not_call: [orders.cancel]
```

Fault targeting: by tool name, `on_call: n`, `every_call`, or `probability`
with a fixed seed. Fault types:
- `timeout`: Hang. Combined with a **per-run wall-clock limit** (default
  5 minutes, configurable) enforced by `run` (Claude's call, after review).
  The Claude Code preset sets a bounded client tool timeout
  (`MCP_TOOL_TIMEOUT`) so the agent's own timeout handling is what gets
  tested, within the run limit. A run killed by the limit fails with reason
  `did-not-finish`.
- `slow`: delay N before forwarding.
- `error`: JSON-RPC error or tool `isError` result (configurable).
- `partial`: forward, then truncate result content (fraction or bytes).
- `malformed`: forward, then corrupt the result (invalid JSON in text
  content, dropped fields, wrong types).

### Grading

Deterministic checks over the trace, exit code and stdout: `max_calls`,
`backoff` (increasing gaps between repeated identical calls),
`must_not_call`, `must_call`, `stops_within` (calls after the fault),
`output_matches` / `output_not_matches`, `exit_code`. An experiment passes if
all expectations hold; with `runs: N` the report shows pass rate per
experiment, because agent behaviour varies run to run.

### Output

Terminal table (as in the README example) and a JSON report containing every
run's trace path, expectations and results. Exit code non-zero when any
experiment's pass rate is below a configurable threshold (default: all runs
pass), so it can gate CI later.

## What "v0 done" means (Claude's call, after review)

Working code locally, not a tagged release. switchyard lives in a sibling
directory (`~/git-openastra/switchyard`, module path
`github.com/sadayamuthu/switchyard`), linked to AgentChaos with `go.work`.
AgentChaos's Go code lives in this repo as `github.com/sadayamuthu/agentchaos`.
The GitHub repo `sadayamuthu/switchyard` now exists and is **private** (user's
choice), so fetching the module outside `go.work` needs
`GOPRIVATE=github.com/sadayamuthu/switchyard`, and CI in consumer repos needs
a token. The licence, making it public and the Homebrew release are a later
step that needs the user. Consumers: agentchaos, intentlock, taintline, fuse,
rewind, airlock (READMEs updated); each adds the Go dependency when it gets
code.

## Proving v0 (agreed with the user)

- Example MCP servers in Go (a fake "CRM" and "orders") in
  `agentchaos/examples/`, with an example experiment suite.
- Automated tests use the `testkit` fake client as the agent: no model calls,
  no API spend, deterministic.
- One manual end-to-end run with Claude Code against the example servers,
  started by the user.

## Left to the builder

- Whether to use the official Go MCP SDK for types/framing or hand-roll.
- Exact Go API names, package layout, YAML schema field names.
- How the debug `switchyard` binary is configured.
- Testing approach beyond: every hook decision path and the transparency
  property must be covered using `testkit`, and one end-to-end test runs a
  chain of two proxies.

## Open, needs the user later (not blocking v0)

- Making `switchyard` public (needed before public READMEs' links work for readers).
- Licence (all READMEs say "to be decided before the first release").
- Python/TS framework wrappers vs proxy-only, when Taintline/Rewind start.
