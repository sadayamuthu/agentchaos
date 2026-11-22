# AgentChaos: thesis

**In one line:** how an agent behaves when its tools break is a property you
can test on every change, and the test harness is a proxy on the tool path.

## The problem

Agents are evaluated on the happy path. In production, tools time out, return
half a record, return malformed data, or return an error the model reads as an
instruction. The failures that hurt are behavioural: retrying forever,
inventing a value to fill a gap, or passing bad data to the next agent. Nobody
finds these until a user does.

## The bet

1. **Faults belong on the tool path, not in mocks.** Wrapping real tools with
   an MCP/HTTP proxy that injects faults exercises the agent exactly as
   production will, with no changes to the agent.
2. **Behaviour can be graded from the trace.** Most unsafe failure modes
   (unbounded retries, no backoff, silent continuation, unexpected tool calls)
   are visible in the sequence of calls, without judging output text.
3. **The proxy is the product's foundation, and OpenAstra's.** The same
   interception layer that injects faults can enforce envelopes (IntentLock),
   track taint (Taintline), count and trip (Fuse) and record undo data
   (Rewind). Building it here, under test pressure, gives every runtime
   project a hardened base.

## Why now

MCP has become the common tool interface across agent clients and
frameworks, so one proxy reaches most agents. Agents are moving from reading
to writing in production systems, which makes failure behaviour a safety
question, not a quality one.

## What success looks like

- A team can point AgentChaos at an existing MCP-based agent and get a
  resilience report in under 15 minutes, with no code changes to the agent.
- Trace-only grading agrees with human review on the core fault library
  (timeout, error, partial, malformed) in the large majority of cases.
- At least two other OpenAstra projects ship on the shared proxy core.
- Teams run it in CI and track the score over time.

## What would prove us wrong

- Agent behaviour under the same fault is so non-deterministic that a single
  run says nothing, and repeated runs are too expensive to be routine.
- The failures that matter can only be graded by a model judge, so results
  aren't reproducible enough to gate a merge.
- Teams don't route tools through a proxy in practice, so the harness can't
  reach real agents.
- The proxy abstraction doesn't fit the needs of IntentLock, Taintline, Fuse
  and Rewind, and each ends up writing its own.

## Not a bet on

Attack testing (that's OpenQA), model quality, or replacing staging
environments.

## Where it fits

Area X, AI Resilience & Recovery. Tests that [Fuse](https://github.com/sadayamuthu/fuse)
trips; uses [Airlock](https://github.com/sadayamuthu/airlock) or staging for
experiments with side effects; supplies the shared proxy core.
