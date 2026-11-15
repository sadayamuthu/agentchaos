# agentchaos

Chaos engineering for AI agents. `agentchaos` deliberately breaks the world
around an agent (tool timeouts, malformed responses, partial failures,
poisoned tool outputs, a peer agent that misbehaves) and reports how the agent
fails: safely, by retrying forever, by making things up to fill the gap, or by
passing the failure on to other agents.

> **Status: design stage.** There is no code yet. The experiment format,
> commands and output below are the proposal, and they will change. Part of
> the [OpenAstra](https://openastra.org) research agenda, area X: AI
> Resilience & Recovery.

## The problem

Agents are tested on the happy path: the tools work, the data is clean, and
the other agents behave. In production, APIs time out, return half a page, or
return an error message the model reads as an instruction. How an agent
behaves when things break matters as much as how it behaves when they don't,
and almost nobody tests it. Chaos engineering made this routine for
distributed systems; agents need the same discipline.

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

## Planned surfaces

- Fault-injecting MCP and HTTP proxy
- Experiment files in YAML, plus a built-in library of common faults
- CLI and a GitHub Action, so resilience is checked on every change
- HTML and JSON reports; scores over time

## Maps to

| Framework | Items |
|-----------|-------|
| OWASP Agentic Top 10 | Cascading failures, memory and context poisoning, insecure inter-agent communication |
| OWASP LLM Top 10 (2025) | LLM09 Misinformation, LLM10 Unbounded Consumption |
| NIST SP 800-53 Rev. 5 | CA-8 Penetration Testing, CP-4 Contingency Plan Testing, SA-11 Developer Testing and Evaluation |

## Design principles

- **Test against real tools, with faults on top**, not against mocks.
- **Grade behaviour, not output text.** What the agent *did* next is what matters.
- **Safe by default.** Experiments that could cause real side effects run in [Airlock](https://github.com/sadayamuthu/airlock) or against staging.

## Roadmap

1. Fault proxy and experiment format
2. Core fault library (timeout, error, partial, malformed)
3. Behaviour classifier and reports
4. Poisoned-output and rogue-peer experiments
5. GitHub Action and trends

## Related OpenAstra projects

- OpenQA (in the lab): adversarial test suites for agents; AgentChaos covers failures rather than attacks
- [Fuse](https://github.com/sadayamuthu/fuse): AgentChaos checks the breakers trip
- [Rewind](https://github.com/sadayamuthu/rewind): undo for experiments run against real systems

## License

To be decided before the first release.
