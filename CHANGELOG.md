# Changelog

## v0.2.0

- `fault: { type: none }` declares a control experiment: no tool, no trigger,
  no other fault fields. The agent runs through the proxy with nothing injected.
- `report.json` has a top-level `provenance` block (agentchaos version, Go
  version, OS/arch, start and finish times).
- Each run in `report.json` records `experiment`, `dir` and `tool_calls`.
- The report is rewritten atomically after every run, so a partial report is
  always valid JSON.
- Timestamps in reports are UTC.

## v0.1.1

- Patch release on the v0.1 line.

## v0.1.0

- First release: the MCP fault-injection proxy, the experiment format and the
  `run` command.
