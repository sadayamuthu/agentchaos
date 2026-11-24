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

## Archived paths

The runner writes absolute paths into `report.json`, each run's `mcp.json`
and the provenance. After the pilot, the archived copies in `data/` were
rewritten to repo-relative paths (for example `./paper/pilot/data/...`,
`./examples/bin/crm-server`). The archived configs are therefore not literally what the agent was given;
apart from the path prefix they are unchanged.

## Provenance after the squash merge

The Sonnet suite's recorded commit, `5c6d1c5` (`v0.2.0-3-g5c6d1c5`), is v0.2.0
plus three commits that change only `paper/` tooling and `.gitignore`. The
branch was squash-merged to keep a large binary blob out of public history, so
`5c6d1c5` is not in the public repository; tag `v0.2.0` is. The `agentchaos`
binary was the same v0.2.0 build for both suites.
