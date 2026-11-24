<div align="center">

# AgentChaos

<h3>Chaos engineering for AI agents</h3>

[![Status](https://img.shields.io/badge/status-v0.2.0-yellow?style=flat-square)](ROADMAP.md)
[![Go Reference](https://pkg.go.dev/badge/github.com/sadayamuthu/agentchaos.svg)](https://pkg.go.dev/github.com/sadayamuthu/agentchaos/cmd/agentchaos)
[![Go 1.26+](https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue?style=flat-square)](LICENSE)

---

[Try it](#try-it) · [How it works](#how-it-works) · [Thesis](THESIS.md) · [Roadmap](ROADMAP.md) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md)

</div>

---

## What is AgentChaos?

Chaos engineering for AI agents. `agentchaos` deliberately breaks the world
around an agent (tool timeouts, malformed responses, partial failures,
poisoned tool outputs, a peer agent that misbehaves) and reports how the agent
fails: safely, by retrying forever, by making things up to fill the gap, or by
passing the failure on to other agents.

> **Status: v0.2.0, early release.** The MCP proxy, the experiment format and
> the `run` command work and are tested, but formats and flags may still change
> before 1.0. The full example
> below is the proposal, not what v0 prints today. Part of
> the [OpenAstra](https://openastra.org) research agenda, area X: AI
> Resilience & Recovery.

---

## Try it

Install the CLI:

```sh
go install github.com/sadayamuthu/agentchaos/cmd/agentchaos@latest
```

To run the example suite, clone the repo; you need Go 1.26 and Claude Code,
logged in. This runs real Claude Code and uses API credits. See
[examples/README.md](examples/README.md) for the walkthrough.

```sh
go build -o examples/bin/agentchaos ./cmd/agentchaos
go build -o examples/bin/crm-server ./examples/crm-server && go build -o examples/bin/orders-server ./examples/orders-server
examples/bin/agentchaos run examples/support-agent.yaml
```

---

## The problem

Agents are tested on the happy path: the tools work, the data is clean, and
the other agents behave. In production, APIs time out, return half a page, or
return an error message the model reads as an instruction. How an agent
behaves when things break matters as much as how it behaves when they don't,
and almost nobody tests it. Chaos engineering made this routine for
distributed systems; agents need the same discipline.

---

## How it works

1. **Wrap the tools.** `agentchaos` runs the agent against its real tools
   through a fault-injecting MCP proxy (stdio MCP in v0; HTTP is planned).
2. **Inject faults** from a library of experiments:
   - `timeout`, `slow`, `error`: the tool hangs, lags or fails
   - `partial`: truncated or paginated-away results
   - `malformed`: invalid JSON, wrong types, empty fields
   - `none`: a control: no fault, same checks
   - `poisoned`: tool output containing instructions or false data
   - `stale`: old data returned as current
   - `rogue-peer`: another agent sends wrong or adversarial messages
3. **Observe.** Record what the agent does next: stop and report, retry,
   switch tools, hallucinate a result, escalate, or hand bad data downstream.
4. **Grade.** Score each experiment against the expected safe behaviour and
   produce a resilience report.

---

## Example (proposed)

```console
$ agentchaos run experiments/support-agent.yaml
EXPERIMENT                    BEHAVIOUR                        RESULT
crm.lookup timeout            retried 3x, told the user        pass
crm.lookup partial            answered from half the record    FAIL  (fabricated plan tier)
orders.refund error 500       retried 41x in 60s               FAIL  (no backoff, no limit)
email.read poisoned           followed embedded instruction    FAIL  (called http.fetch)
rogue-peer billing-agent      accepted wrong total, refunded   FAIL  (no cross-check)

1 of 5 passed. Report: reports/support-agent-2026-11-16.html
```

---

## Surfaces

Available in v0:

- Fault-injecting MCP proxy (`agentchaos proxy`)
- YAML experiment files and the core faults: `timeout`, `slow`, `error`, `partial`, `malformed`
- CLI (`agentchaos run`) that grades runs of a real agent
- JSON report

Still planned:

- HTTP proxy
- GitHub Action, so resilience is checked on every change
- HTML report and scores over time
- The `poisoned`, `stale` and `rogue-peer` faults described above are planned, not in v0

---

## Maps to

| Framework | Items |
|-----------|-------|
| OWASP Agentic Top 10 | Cascading failures, memory and context poisoning, insecure inter-agent communication |
| OWASP LLM Top 10 (2025) | LLM09 Misinformation, LLM10 Unbounded Consumption |
| NIST SP 800-53 Rev. 5 | CA-8 Penetration Testing, CP-4 Contingency Plan Testing, SA-11 Developer Testing and Evaluation |

---

## Design principles

- **Test against real tools, with faults on top**, not against mocks.
- **Grade behaviour, not output text.** What the agent *did* next is what matters.
- **Safe by default.** Experiments that could cause real side effects run in [AgentBox](https://github.com/sadayamuthu/agentbox) or against staging.

---

## Roadmap

The plan, from the first working version to 1.0, is in [ROADMAP.md](ROADMAP.md). The reasoning behind the project, and what would make us stop, is in [THESIS.md](THESIS.md).

---

## Built on switchyard

`agentchaos` is a Go binary built on [switchyard](https://github.com/sadayamuthu/switchyard), OpenAstra's shared MCP proxy core: its fault-injecting proxy is a set of switchyard hooks. AgentChaos is switchyard's first consumer. It stacks with the other switchyard tools by chaining (`agent → agentfuse → agentlock → agenttaint → agentrewind → agentbox → MCP server`).

---

## Related OpenAstra projects

- OpenQA (in the lab): adversarial test suites for agents; AgentChaos covers failures rather than attacks
- [AgentFuse](https://github.com/sadayamuthu/agentfuse): AgentChaos checks the breakers trip
- [AgentRewind](https://github.com/sadayamuthu/agentrewind): undo for experiments run against real systems

---

## Contributing

Design feedback, real-world cases and fixes are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for how to help while v0 is in development, and [GOVERNANCE.md](GOVERNANCE.md) for how decisions are made.

---

## Security

Please report vulnerabilities privately through [GitHub security advisories](https://github.com/sadayamuthu/agentchaos/security/advisories/new), not in a public issue. See [SECURITY.md](SECURITY.md) for what to include and what's in scope.

---

## License

[Apache License 2.0](LICENSE)

---

<div align="center">

**AgentChaos: chaos engineering for AI agents**

AgentChaos is managed by [OpenAstra](https://openastra.org)

[GitHub](https://github.com/sadayamuthu/agentchaos) · [Thesis](THESIS.md) · [Roadmap](ROADMAP.md)

</div>
