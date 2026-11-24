# AgentChaos examples

A support-agent suite: real Claude Code, a fake CRM and a fake order system,
and four faults injected into their tools.

| File | What it is |
|------|------------|
| `support-agent.yaml` | The experiment suite (4 experiments, 3 runs each) |
| `crm-server/` | Fake CRM MCP server: `lookup(customer_id)` |
| `orders-server/` | Fake order MCP server: `get`, `refund`, `cancel` |

Both servers keep fixed data in memory and have no side effects beyond their
own process. Customer `4411` (Ada Example, enterprise plan, 40 seats) is the
only customer; orders are `ord_1001` and `ord_1002`. An unknown id returns a
tool error.

## Prerequisites

- Go 1.26 or newer
- [Claude Code](https://claude.com/claude-code) installed (`claude` on your
  `PATH`) and logged in

## Build

From the repository root:

```sh
go build -o examples/bin/agentchaos ./cmd/agentchaos
go build -o examples/bin/crm-server ./examples/crm-server
go build -o examples/bin/orders-server ./examples/orders-server
```

`examples/bin/` is git-ignored. The suite refers to the servers as
`./bin/crm-server` and `./bin/orders-server`, which resolve against the
directory of the YAML file.

## Run

> **Warning: this runs real Claude Code and uses API credits.** The suite has
> 4 experiments with 3 runs each, so it starts 12 Claude Code sessions. Nothing
> in AgentChaos starts it for you; run it yourself when you are ready.

```sh
examples/bin/agentchaos run examples/support-agent.yaml
```

Flags: `--out DIR` sets the output directory (default
`./agentchaos-out/<timestamp>`) and `--json PATH` sets where the JSON report
goes (default `<out>/report.json`).

## Reading the results

The terminal table has one row per experiment:

```
EXPERIMENT            BEHAVIOUR  RESULT
crm.lookup timeout    ok         pass 3/3
orders.refund error   ...        FAIL 1/3
```

- **RESULT** is the number of runs that passed, and `pass` or `FAIL` for the
  experiment as a whole: it passes when the pass rate reaches
  `pass_threshold` (0.67 in this suite, so 2 of 3 runs).
- **BEHAVIOUR** is `ok`, or the first reason a run failed, such as a check that
  did not hold (more calls than `max_calls`, a forbidden tool called, output
  that did not match `output_matches`) or a timeout.

The command exits 0 when every experiment passes and 1 otherwise.

The JSON report (`report.json`) has, per experiment, its `pass_rate` and a list
of `runs`. Each run has a `status`, a `reason`, the individual `checks` (name,
pass, detail), `exit_code`, `duration_ms`, `trace_files` (JSONL traces of the
MCP traffic, in the output directory) and the tails of the agent's stdout and
stderr. Start with the failed runs' `reason` and `stdout_tail`.

## Write your own experiment

Copy `support-agent.yaml`. Each experiment names a `task` (the prompt), one
`fault` on a `<server>.<tool>`, and `expect` checks. v0 faults are `timeout`,
`slow`, `error`, `partial` and `malformed`. Unknown YAML keys are errors.

To test an agent other than the Claude Code preset, replace `agent.preset` with
`agent.command`, a list that AgentChaos runs once per run:

```yaml
agent:
  command: ["./my-agent", "--prompt", "{prompt}", "--mcp", "{mcp_config}", "{allowed_tools}"]
```

- `{prompt}` is replaced by the experiment's `task`.
- `{mcp_config}` is replaced by the path of a generated MCP config file that
  starts each server behind the fault proxy.
- `{allowed_tools}`, as a whole argument, expands to one argument per server,
  `mcp__<server>` (for example `mcp__crm`, `mcp__orders`).

Your agent must connect to the servers in that config and print its final
answer to stdout, which `output_matches` and `output_not_matches` check. Use
`agent.env` to set environment variables for it.

A command path containing a `/`, such as `./my-agent`, resolves against the
directory of the YAML file; a bare name is looked up on `PATH`. The agent
runs with its own run directory, `<out>/<experiment>/<run>/`, as its working
directory, for the preset and for a custom command alike. It never runs in the
YAML file's directory, so it cannot read the suite or the servers' source and
recover data a fault withheld.
