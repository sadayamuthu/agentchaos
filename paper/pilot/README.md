# Pilot

A 160-session pilot: 2 models x 8 experiments (4 faults, each with a no-fault
control) x 10 runs. Suites are in `suites/`; `run.sh` drives one suite.

## Replicating

From the repository root, at tag `v0.2.0`:

```sh
go build -o examples/bin/agentchaos ./cmd/agentchaos
go build -o examples/bin/crm-server ./examples/crm-server
go build -o examples/bin/orders-server ./examples/orders-server

# 2 real sessions to check the setup (output is git-ignored)
paper/pilot/run.sh paper/pilot/suites/smoke.yaml paper/pilot/smoke-out

paper/pilot/run.sh paper/pilot/suites/sonnet.yaml paper/pilot/data/sonnet
paper/pilot/run.sh paper/pilot/suites/haiku.yaml paper/pilot/data/haiku
```

`run.sh` refuses to reuse an output directory. Each output directory holds
`report.json`, `provenance.txt` (commit, `git describe`, agentchaos and claude
versions, model, UTC start and end) and one directory per run with traces.

Requires the `claude` CLI, logged in. Models: `claude-sonnet-5-5`,
`claude-haiku-5-5`. Agent runs use `MCP_TOOL_TIMEOUT=30000`.

## Controls

A control has the same task and `expect` as its fault twin but
`fault: { type: none }`. Expectations such as `output_matches` for an error
message are written for the faulted run, so a control will usually report
`fail` on them. Read controls for tool-call counts and agent behaviour without
a fault, not for pass/fail.
