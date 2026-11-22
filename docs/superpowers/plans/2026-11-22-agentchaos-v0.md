# AgentChaos v0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An `agentchaos` binary that injects tool faults through switchyard and grades how an agent behaves, from experiment files.

**Architecture:** `agentchaos proxy` is a switchyard proxy with one fault hook, wrapping one upstream MCP server. `agentchaos run` generates an MCP config that wraps every server in `agentchaos proxy`, runs the agent command per experiment × run with a wall-clock limit, merges the per-server JSONL traces, grades them deterministically and prints a table plus a JSON report.

**Tech Stack:** Go 1.26, `github.com/sadayamuthu/switchyard` (local via `go.work`), `go.yaml.in/yaml/v3` v3.0.6, `github.com/modelcontextprotocol/go-sdk` v1.8.0 (example servers, fake agent, tests).

**Spec:** `docs/superpowers/specs/2026-11-22-switchyard-agentchaos-v0-design.md`

**Prerequisite:** switchyard plan (`~/git-openastra/switchyard/docs/superpowers/plans/2026-11-22-switchyard-v0.md`) complete. This plan uses its `Hook`, `Base`, `Decision` constructors and `Annotate`, `Session.Update`, `Response.ToolResult`, `ToolResult.Marshal`, `Event`, `NewJSONLSink`, `ChainIDFromEnv`, `RunCommand`, and `testkit`.

## Global Constraints

- Module `github.com/sadayamuthu/agentchaos`; `go 1.26`.
- `~/git-openastra/go.work` with `use ./switchyard ./agentchaos` (that directory is not a repo; the file is not committed anywhere). `agentchaos/go.mod` requires `github.com/sadayamuthu/switchyard v0.0.0-00010101000000-000000000000`.
- Fault types exactly: `timeout`, `slow`, `error`, `partial`, `malformed`.
- Per-run wall-clock limit default `5m`; killed runs fail with reason `did-not-finish`.
- Claude Code preset sets `MCP_TOOL_TIMEOUT=30000` in the agent's environment.
- Grading is deterministic: no model calls anywhere in v0, including tests.
- Automated tests use the fake agent, never real Claude Code.
- Out of scope: HTTP transport, `poisoned`/`stale`/`rogue-peer`, model judge, GitHub Action, HTML report, Airlock.

## Review Focus

1. **Experiment names a server or tool that doesn't exist:** fail validation before running anything (exit 2), naming the bad reference. Unknown tool can only be checked at run time → see 2. Test in Task 1.
2. **Fault never triggers during a run** (agent never called the tool): the run is `invalid` with reason `fault-not-triggered`, never `pass`. Test in Task 5.
3. **Agent command not found, or exits immediately non-zero:** that run is `error` with the exec error or exit code and stderr tail; other runs continue. Test in Task 5.
4. **Relative paths** in `servers.*.command` and `agent.command` resolve against the experiment file's directory, not the current directory. Test in Task 1.
5. **Prompts with quotes, newlines, `$`:** passed as one argv element, never through a shell. Test in Task 5.

---

## File Structure

```
agentchaos/
  go.mod
  cmd/agentchaos/main.go           subcommand dispatch: proxy, run, version
  internal/experiment/schema.go    types + defaults
  internal/experiment/load.go      Load, Validate, path resolution
  internal/faults/hook.go          switchyard hook applying one Fault
  internal/faults/mutate.go        truncate / corrupt tool results
  internal/proxycmd/proxycmd.go    `agentchaos proxy`
  internal/runner/runner.go        per-run orchestration
  internal/runner/agent.go         argv templating, presets
  internal/grade/grade.go          checks over trace/stdout/exit
  internal/report/report.go        table + JSON, exit code
  internal/fakeagent/main.go       scripted MCP client used as the agent in tests
  examples/crm-server/main.go      fake CRM MCP server
  examples/orders-server/main.go   fake orders MCP server
  examples/support-agent.yaml      example suite
```

---

### Task 1: Module, workspace, experiment schema

**Files:**
- Create: `go.mod`, `~/git-openastra/go.work`, `internal/experiment/schema.go`, `internal/experiment/load.go`
- Test: `internal/experiment/load_test.go`, `internal/experiment/testdata/*.yaml`

**Interfaces:**
- Produces:
  - `type File struct { Path string; Agent Agent; Servers map[string]Server; Runs int; RunTimeout time.Duration; PassThreshold float64; Experiments []Experiment }` — YAML keys `agent`, `servers`, `runs` (default 1), `run_timeout` (default `5m`), `pass_threshold` (default `1.0`), `experiments`.
  - `type Agent struct { Preset string; Command []string; Env map[string]string }` — exactly one of `preset` (only `claude-code` in v0) or `command`.
  - `type Server struct { Command []string; Env map[string]string }`
  - `type Experiment struct { Name, Task string; Fault Fault; Expect Expect }`
  - `type Fault struct { Tool string; Type string; OnCall int; EveryCall bool; Probability float64; Seed int64; Delay time.Duration; ErrorMode string; Message string; Fraction float64; Bytes int; Corrupt string }` — YAML: `tool` (`<server>.<tool>`), `type`, `on_call`, `every_call`, `probability`, `seed`, `delay`, `error_mode` (`tool` default | `jsonrpc`), `message` (default `"internal error"`), `fraction` (partial; default 0.5), `bytes`, `corrupt` (`invalid-json` default | `drop-fields` | `wrong-types`).
  - `func (f Fault) Server() string`, `func (f Fault) ToolName() string` — split on first `.`.
  - `type Expect struct { MaxCalls *MaxCalls; Backoff bool; MustCall, MustNotCall []string; StopsWithin *int; OutputMatches, OutputNotMatches string; ExitCode *int }`; `type MaxCalls struct { Tool string; Count int }`. YAML keys as in the spec example plus `must_call`, `stops_within`, `output_not_matches`, `exit_code`.
  - `func Load(path string) (*File, error)` — parses, applies defaults, resolves relative command paths (first element only, if it contains a `/`) against `filepath.Dir(path)`, calls `Validate`.
  - `func (f *File) Validate() error` — returns all problems joined.

**Validation rules:** server keys non-empty, no `.`; `fault.tool` server exists; `fault.type` in the five; exactly one trigger among `on_call>0`, `every_call`, `probability` in (0,1]; `slow` requires `delay>0`; `max_calls.tool`, `must_call`, `must_not_call` reference existing servers; regexes compile; experiment names unique; `runs>=1`; `0<pass_threshold<=1`.

- [ ] **Step 1: Create go.mod and go.work; `go build ./...` in `~/git-openastra` succeeds with switchyard resolved locally.**
- [ ] **Step 2: Write failing tests:** `TestLoadSpecExample` (the spec's YAML example loads; `Runs==5`, `RunTimeout==5m`, fault server `crm`, tool `lookup`); `TestValidateErrors` table — unknown server, bad type, two triggers, `slow` without delay, bad regex, duplicate name, server key with dot — each error message names the offending field; `TestRelativePaths` (`./crm-server` in `testdata/sub/x.yaml` → absolute path under `testdata/sub`).
- [ ] **Step 3: Run `go test ./internal/experiment/...` — expect FAIL.**
- [ ] **Step 4: Implement.**
- [ ] **Step 5: Run — expect PASS.**
- [ ] **Step 6: Commit** `feat: experiment file schema, defaults and validation`

---

### Task 2: Fault hook

**Files:**
- Create: `internal/faults/hook.go`, `internal/faults/mutate.go`
- Test: `internal/faults/hook_test.go`

**Interfaces:**
- Consumes: `experiment.Fault`; switchyard `Hook`, `Base`, decisions, `Annotate`, `Session.Update`, `Response.ToolResult`, `testkit`.
- Produces: `func New(f experiment.Fault) switchyard.Hook` — `Name()` returns `agentchaos`. Applies only to `tools/call` whose tool name equals `f.ToolName()`.

**Behaviour (exact):**
- Per-session counter of calls to the target tool via `Session.Update`. Triggers: `on_call: n` → nth call only; `every_call` → all; `probability` → `rand.New(rand.NewPCG(uint64(seed), 0))` drawn once per target call, in call order.
- Triggered request decisions, all annotated `fault-<type>`:
  - `timeout` → `Hang()`
  - `slow` → sleep `Delay` (respecting ctx), then `Continue()`
  - `error` → `BlockTool(Message)` if `ErrorMode=="tool"`, else `Block(&jsonrpc.Error{Code:-32603, Message:Message})`
  - `partial`, `malformed` → `Continue()`, and mark the request ID in the session so `OnResponse` mutates it.
- `partial`: each text content item truncated to `Bytes` if >0, else `floor(len*Fraction)` bytes, cut on a UTF-8 boundary; non-text items left untouched.
- `malformed`: `invalid-json` → each text item's text replaced by its first half plus `{"` (unterminated); `drop-fields` → remove `content`, keep other fields; `wrong-types` → `content` replaced with the string `"content"`.

- [ ] **Step 1: Write failing tests** (testkit server tool `lookup` returns text `{"plan":"enterprise","seats":40}`; second tool `other`):
  - `TestTimeoutHangs`: on_call 1; first call with 200 ms ctx times out; second call succeeds.
  - `TestSlowDelays`: delay 300 ms; call takes ≥300 ms and returns normal result.
  - `TestErrorToolMode` / `TestErrorJSONRPCMode`: IsError result with message / call error with message.
  - `TestPartialFraction`: fraction 0.5 → text is the first half of the original.
  - `TestMalformedInvalidJSON`: text is not valid JSON.
  - `TestOtherToolUntouched`: `other` never faulted.
  - `TestProbabilitySeedReproducible`: probability 0.5 seed 42, 20 calls, run twice → identical faulted-call index sets.
  - `TestDecisionAnnotated`: with `testkit.Recorder`, request event decision is `hang:fault-timeout`.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run `go test -race ./internal/faults/...` — expect PASS.**
- [ ] **Step 5: Commit** `feat: fault hook for timeout, slow, error, partial, malformed`

---

### Task 3: `agentchaos proxy` subcommand

**Files:**
- Create: `cmd/agentchaos/main.go`, `internal/proxycmd/proxycmd.go`
- Test: `internal/proxycmd/proxycmd_test.go`

**Interfaces:**
- Produces: `func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int` and CLI:
  `agentchaos proxy --experiment FILE --name EXPERIMENT --server SERVER [--trace FILE] -- <cmd> [args...]`
  — loads the file, finds the experiment; installs `faults.New(fault)` only if `fault.Server()==SERVER`, otherwise runs as pure pass-through; hop name = SERVER; trace via `NewJSONLSink`; upstream via `switchyard.RunCommand`. Exit 2 on bad flags or unknown experiment.
- `cmd/agentchaos/main.go`: stdlib `flag`, subcommands `proxy`, `run` (Task 6), `version`.

- [ ] **Step 1: Write failing tests** (build `agentchaos` and switchyard's `internal/testserver`-equivalent: add `internal/testserver/main.go` here with tools `lookup`, `other` using the SDK):
  - `TestProxyInjectsFault`: SDK client via `CommandTransport` running `agentchaos proxy ... -- testserver` with an `error` fault on `crm.lookup`, `--server crm`; `lookup` returns IsError; trace file has a `block-tool:fault-error` event.
  - `TestProxyOtherServerPassThrough`: same experiment, `--server orders`; `lookup` succeeds.
  - `TestProxyUnknownExperiment`: exit code 2, stderr names the experiment.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run — expect PASS.**
- [ ] **Step 5: Commit** `feat: agentchaos proxy subcommand`

---

### Task 4: Grading

**Files:**
- Create: `internal/grade/grade.go`
- Test: `internal/grade/grade_test.go`

**Interfaces:**
- Consumes: `experiment.Expect`, `switchyard.Event`.
- Produces:
  - `type Check struct { Name string; Pass bool; Detail string }`
  - `func Evaluate(e experiment.Expect, events []switchyard.Event, stdout string, exitCode int) []Check` — `events` are all hops' events sorted by `Time`; a tool call is a `request` event with `Method=="tools/call"`, identified as `<Hop>.<Tool>`.

**Definitions (exact):**
- `max_calls`: calls to tool ≤ count. Detail: `"called 41 times, max 3"`.
- `must_call` / `must_not_call`: each listed tool called ≥1 / 0 times.
- `stops_within: n`: calls (any tool) after the first event whose `Decision` contains `:fault-` ≤ n.
- `backoff`: for each tool, take consecutive calls with byte-identical `Params`; if ≥3, the gaps must be non-decreasing within 10% tolerance (`gap[i] >= 0.9*gap[i-1]`) and `last gap > first gap`. Fewer than 3 → pass with detail `"fewer than 3 identical calls"`.
- `output_matches` / `output_not_matches`: Go `regexp` against stdout.
- `exit_code`: equality.
- Only expectations that are set produce checks.

- [ ] **Step 1: Write failing tests**, one per check with pass and fail cases built from synthetic events, including: backoff with gaps 1s,1s,1s → fail; 1s,2s,4s → pass; 1s,0.95s,2s → pass (tolerance); 2 identical calls → pass; stops_within counting only events after the first fault.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run — expect PASS.**
- [ ] **Step 5: Commit** `feat: deterministic behaviour grading`

---

### Task 5: Runner and fake agent

**Files:**
- Create: `internal/runner/runner.go`, `internal/runner/agent.go`, `internal/fakeagent/main.go`
- Test: `internal/runner/runner_test.go`

**Interfaces:**
- Consumes: Tasks 1–4.
- Produces:
  - `type RunResult struct { Experiment string; Index int; Status string; Reason string; Checks []grade.Check; ExitCode int; Duration time.Duration; TraceFiles []string; StdoutTail, StderrTail string }` — `Status` ∈ `pass`, `fail`, `invalid`, `error`. `Reason` ∈ `did-not-finish`, `fault-not-triggered`, `agent-error`, or empty.
  - `type Runner struct { File *experiment.File; Self string /* path to agentchaos binary */; WorkDir string }`
  - `func (r *Runner) RunAll(ctx context.Context) []RunResult` — experiments in order, each `Runs` times, sequentially.
  - `func BuildArgv(a experiment.Agent, prompt, mcpConfig string, servers []string) ([]string, map[string]string)` — substitutes `{prompt}` and `{mcp_config}` inside elements; an element exactly `{allowed_tools}` expands to one element per server, `mcp__<server>`. Preset `claude-code` = `["claude","-p","{prompt}","--mcp-config","{mcp_config}","--strict-mcp-config","--allowedTools","{allowed_tools}"]` with env `MCP_TOOL_TIMEOUT=30000`.

**Per run:** create `WorkDir/<exp-slug>/<index>/`; write `mcp.json` = `{"mcpServers":{"<server>":{"command":Self,"args":["proxy","--experiment",File.Path,"--name",exp,"--server",server,"--trace","<dir>/trace-<server>.jsonl","--",cmd...],"env":server.Env}}}`; run argv with `exec.CommandContext` under `RunTimeout` (kill process group on timeout), cwd = experiment file dir, env = os env + agent env + `SWITCHYARD_CHAIN_ID` fresh per run; read and merge traces; status order: start failure or non-zero exit with no tool calls → `error/agent-error`; timeout → `fail/did-not-finish`; no `:fault-` decision in trace → `invalid/fault-not-triggered`; else `pass` iff all checks pass.

**Fake agent** (`internal/fakeagent`): flags `--mcp-config PATH --prompt S --call server.tool --args JSON --max-attempts N --backoff none|exp --base 100ms --on-failure report|silent|hang --print-prompt`. Connects to the named server from the config via `CommandTransport`, calls the tool up to N times until a non-error result, sleeping per backoff; on final failure prints `unable to complete: <reason>` (report), nothing (silent), or blocks forever (hang); on success prints the result text; `--print-prompt` echoes the prompt.

- [ ] **Step 1: Write failing tests** (build `agentchaos`, `fakeagent`, `internal/testserver`; experiments use `agent.command` invoking fakeagent):
  - `TestRunPass`: error fault every_call, fakeagent max-attempts 3 backoff exp on-failure report; expect max_calls 3, backoff, output_matches `unable`; → `pass`.
  - `TestRunFailNoBackoff`: same with `--backoff none --max-attempts 5`, expect max_calls 3 → `fail`, check detail `called 5 times, max 3`.
  - `TestRunDidNotFinish`: timeout fault, on-failure hang, `run_timeout: 2s` → `fail/did-not-finish` within 5 s.
  - `TestRunFaultNotTriggered`: fault on `crm.other`, agent calls `lookup` → `invalid/fault-not-triggered`.
  - `TestRunAgentMissing`: agent command `./does-not-exist` → `error/agent-error`; next experiment still runs.
  - `TestPromptArgvSafe`: prompt `it's "$HOME"\nline2` with `--print-prompt` → stdout contains it verbatim.
  - `TestBuildArgvClaudePreset`: servers `crm`,`orders` → argv ends with `--allowedTools mcp__crm mcp__orders`; env has `MCP_TOOL_TIMEOUT=30000`.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run `go test -race ./internal/runner/...` — expect PASS.**
- [ ] **Step 5: Commit** `feat: experiment runner with wall-clock limit and fake agent`

---

### Task 6: `agentchaos run` and report

**Files:**
- Create: `internal/report/report.go`
- Modify: `cmd/agentchaos/main.go`
- Test: `internal/report/report_test.go`, `cmd/agentchaos/run_test.go`

**Interfaces:**
- Produces:
  - `agentchaos run FILE [--out DIR] [--json PATH]` — `--out` default `./agentchaos-out/<timestamp>`; JSON report default `<out>/report.json`.
  - `func Table(w io.Writer, f *experiment.File, results []runner.RunResult)` — columns `EXPERIMENT  BEHAVIOUR  RESULT` as in the README example; with `runs>1` RESULT is `pass 4/5` or `FAIL 2/5`; BEHAVIOUR is the first failing check's detail, else `ok`; final line `N of M passed. Report: <path>`.
  - `func WriteJSON(path string, f *experiment.File, results []runner.RunResult) error` — `{"version":1,"file":...,"experiments":[{"name":...,"pass_rate":0.8,"runs":[RunResult...]}]}`.
  - `func ExitCode(f *experiment.File, results []runner.RunResult) int` — 0 if every experiment's pass rate ≥ `PassThreshold` (`invalid` and `error` runs count as not passed), else 1; config errors exit 2 (from `Load`).

- [ ] **Step 1: Write failing tests:** table golden for one passing and one failing experiment with runs 5; JSON round-trips with `version: 1`; `ExitCode` thresholds (4/5 with 0.8 → 0, with 1.0 → 1); CLI test running Task 5's `TestRunPass` file end to end → exit 0 and report file exists; invalid YAML → exit 2.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run `go test -race ./... && go vet ./...` — expect PASS.**
- [ ] **Step 5: Commit** `feat: agentchaos run with table and JSON report`

---

### Task 7: Example servers, suite, docs

**Files:**
- Create: `examples/crm-server/main.go`, `examples/orders-server/main.go`, `examples/support-agent.yaml`, `examples/README.md`
- Modify: `README.md` (status line and a "Try it" section)
- Test: `examples/examples_test.go`

**Interfaces:**
- `crm-server` tools: `lookup(customer_id)` → `{"customer_id":4411,"name":"Ada Example","plan":"enterprise","seats":40,"email":"ada@example.com"}` for 4411, tool error otherwise.
- `orders-server` tools: `get(order_id)`, `refund(order_id, amount)` → `{"refund_id":"rf_1","status":"refunded"}`, `cancel(order_id)`. In-memory, no side effects beyond the process.
- `support-agent.yaml`: `agent.preset: claude-code`, `runs: 3`, the four experiments from the README example that v0 supports (crm lookup timeout, crm lookup partial, orders refund error, plus orders get malformed), with expectations.

- [ ] **Step 1: Write failing test** `TestExampleSuiteValidates`: `experiment.Load("support-agent.yaml")` succeeds; `TestExampleServersRespond`: SDK client calls `crm lookup 4411` and `orders refund` through `agentchaos proxy` and gets the documented results.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement servers and suite; `examples/README.md` documents building the servers and the manual run: `agentchaos run examples/support-agent.yaml` (uses real Claude Code and API credits; started by the user).**
- [ ] **Step 4: Run `go test -race ./...` — expect PASS.**
- [ ] **Step 5: Update README status from "design stage" to "v0 in development", add "Try it" pointing to `examples/README.md`.**
- [ ] **Step 6: Commit** `feat: example MCP servers and support-agent suite`
- [ ] **Step 7: Hand off to the user for the manual Claude Code run; do not run it automatically.**
