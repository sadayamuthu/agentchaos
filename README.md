<div align="center">

# AgentChaos

<h3>Chaos engineering for AI agents</h3>

[![Status](https://img.shields.io/badge/status-design%20stage-orange?style=flat-square)](ROADMAP.md)
[![Go 1.26+](https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue?style=flat-square)](LICENSE)

---

[How it works](#how-it-works) · [Thesis](THESIS.md) · [Roadmap](ROADMAP.md) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md)

</div>

---

## What is AgentChaos?

Chaos engineering for AI agents. `agentchaos` deliberately breaks the world
around an agent (tool timeouts, malformed responses, partial failures,
poisoned tool outputs, a peer agent that misbehaves) and reports how the agent
fails: safely, by retrying forever, by making things up to fill the gap, or by
passing the failure on to other agents.

> **Status: design stage.** There is no code yet. The experiment format,
> commands and output below are the proposal, and they will change. Part of
> the [OpenAstra](https://openastra.org) research agenda, area X: AI
> Resilience & Recovery.

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
   through a fault-injecting proxy (MCP or HTTP).
2. **Inject faults** from a library of experiments:
   - `timeout`, `slow`, `error`: the tool hangs, lags or fails
   - `partial`: truncated or paginated-away results
   - `malformed`: invalid JSON, wrong types, empty fields
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

## Planned surfaces

- Fault-injecting MCP and HTTP proxy
- Experiment files in YAML, plus a built-in library of common faults
- CLI and a GitHub Action, so resilience is checked on every change
- HTML and JSON reports; scores over time

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
- **Safe by default.** Experiments that could cause real side effects run in [Airlock](https://github.com/sadayamuthu/airlock) or against staging.

---

## Roadmap

The plan, from the first working version to 1.0, is in [ROADMAP.md](ROADMAP.md). The reasoning behind the project, and what would make us stop, is in [THESIS.md](THESIS.md).

---

## Built on switchyard

`agentchaos` will be a Go binary built on [switchyard](https://github.com/sadayamuthu/switchyard), OpenAstra's shared MCP proxy core: its fault-injecting proxy is a set of switchyard hooks. AgentChaos is switchyard's first consumer. It stacks with the other switchyard tools by chaining (`agent → fuse → intentlock → taintline → rewind → MCP server`).

---

## Related OpenAstra projects

- OpenQA (in the lab): adversarial test suites for agents; AgentChaos covers failures rather than attacks
- [Fuse](https://github.com/sadayamuthu/fuse): AgentChaos checks the breakers trip
- [Rewind](https://github.com/sadayamuthu/rewind): undo for experiments run against real systems

---

## Contributing

Design feedback, real-world cases and fixes are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for how to help while the project is at the design stage, and [GOVERNANCE.md](GOVERNANCE.md) for how decisions are made.

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
