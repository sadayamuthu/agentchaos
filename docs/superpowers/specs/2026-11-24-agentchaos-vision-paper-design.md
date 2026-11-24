# AgentChaos vision paper with pilot study: design

Date: 2026-11-24. Status: draft for approval.

## Intent

Establish chaos engineering for AI agents as a research discipline, with
AgentChaos as its reference implementation. The goal is **academic
credibility**: a paper other researchers can cite and build on. It comes
first; it also lays groundwork for the book *Breaking Agents on Purpose:
Chaos Engineering for AI Systems* and for a later full empirical paper.

"Harden" here means two things: the method must survive peer review, and the
pilot must exercise the tool against a real agent, so weaknesses it exposes
get fixed before anyone else relies on it.

## Goals

- An 8–10 page vision paper (ACM `acmart`, `sigconf` layout), built to PDF
  in this repo.
- Posted first to arXiv (cs.SE, cross-list cs.AI) to timestamp the idea and
  make it citable.
- Then submitted to a new-ideas track or an AI-for-SE workshop at ICSE or
  FSE; the venue is picked when the draft is done, against current deadlines.
- A small pilot study with real numbers, so the paper reports evidence and
  not only a proposal.

## Non-goals

- Not the full empirical study. That is the next paper; this one states its
  protocol.
- Not a practitioner white paper or marketing document.
- Not the book.

## Anti-goals: failure even if it "works"

- **Any made-up citation.** Every reference must be a real, checkable work
  with correct authors, venue and year. Anything that can't be verified gets
  cut.
- Claims the pilot doesn't support: no "agents fail X% of the time" from one
  agent and four experiments.
- Overselling v0. Planned faults (`poisoned`, `stale`, `rogue-peer`) are
  described as future work, not results.

## Approaches considered

- **A. Vision paper with a pilot study (chosen).** Credible at a reviewed
  venue, modest cost, and doubles as the hardening exercise.
- B. Vision paper with no pilot. Faster and free, but a weak submission.
- C. Industry white paper. Good for adoption, little academic weight; doesn't
  fit the goal.

## Paper structure

1. **Introduction.** Agents are tested on the happy path, but production
   tools fail. Contributions:
   - a failure model for tool-using agents
   - a method: faults on the tool path, deterministic grading from the call
     trace, repeated runs
   - an open-source implementation
   - a pilot study and a research agenda
2. **Background and related work.**
   - Chaos engineering (Netflix; Basiri et al., IEEE Software 2016; Principles
     of Chaos Engineering)
   - Software fault injection
   - LLM agents and tool use (e.g. ReAct)
   - MCP
   - Agent benchmarks and tool-risk work (e.g. τ-bench, AgentBench, ToolEmu)
   - What's new here: real tools with faults on top, graded on behaviour, not
     benchmark task success
3. **A failure model for tool-using agents.** Two axes:
   - injected faults: timeout, slow, error, partial, malformed (v0); poisoned,
     stale, rogue-peer (planned)
   - unsafe behaviours: unbounded retry, no backoff, fabrication filling a gap,
     silent continuation, wrong or unsafe tool, passing bad data downstream
4. **Method.**
   - Chaos engineering's steady-state hypothesis, adapted to agents: the
     expected safe behaviour.
   - Faults injected by a transparent MCP proxy, with no changes to the agent.
   - Deterministic targeting: per-tool arrival index and a seeded probability.
   - Trace-only grading predicates: max_calls, backoff, must_call,
     must_not_call, stops_within and output patterns.
   - Repeated runs with a pass threshold, to handle non-determinism.
   - A clear separation of pass / fail / invalid / error.
5. **Implementation.**
   - switchyard: a shared hook-based proxy core.
   - AgentChaos on top of it.
   - Chaining with other runtime tools.
   - Process isolation, and the agent sandboxed from the answers.
6. **Pilot study.** Setup, research questions, results, discussion (see
   below).
7. **Research agenda.** The full-study protocol:
   - more agents, frameworks and models
   - human-agreement labelling for trace grading
   - the planned faults
   - CI adoption
8. **Threats to validity and ethics.**
   - The pilot is a single domain and fake servers.
   - Regex output checks are crude.
   - Model versions drift over time.
   - Experiments with side effects must run in staging or a sandbox.
9. **Conclusion.**

## Pilot study

**Research questions** (taken from THESIS.md, "what would prove us wrong"):

- **RQ1, repeatability.** Under the same fault, how consistent is agent
  behaviour across runs? Measure the pass-rate spread and the variation in
  the call sequences.
- **RQ2, failure modes.** Which unsafe behaviours appear, for which faults?
  How do they differ between models?
- **RQ3, grading agreement (pilot-scale).** Do the trace-based verdicts match
  a human reading of the same transcripts? The full study does this at scale;
  the pilot reports a small labelled sample.

**Design** (my proposal, open to change):

- **Agent:** Claude Code headless, with two models (for example a Sonnet-class
  and a Haiku-class model), through the custom `command` template so the model
  can be set.
- **Experiments:** the 4 in `examples/support-agent.yaml`, plus 4 matching
  no-fault controls, so any behaviour change can be attributed to the fault.
- **Runs:** 10 per cell, so 8 × 10 × 2 = **160 sessions**.
- **Data:** raw traces, transcripts, report JSON, the agent and model versions,
  and the dates. All archived in the repo so the numbers can be checked.

**Tool hardening this may require** (to be confirmed while building):

- A way to run a no-fault control. Today every experiment needs a fault, and
  a fault that never fires is graded `invalid`.
- An aggregate or export step for per-run data across suites.
- Fixes for whatever breaks on first real use.

### Calls made after the builder check (mine, open to change)

- **Controls reuse the same predicates.**
  - A control is the same task with no fault (this needs `fault: none` in the
    schema; the trigger check is skipped).
  - A control that fails its fault-aware predicates measures how often the
    grader flags a healthy run: its baseline false-positive rate.
  - Controls also supply the baseline call counts and call sequences that the
    fault runs are compared against.
  - So RQ1 and RQ2 are fault-vs-control comparisons, not just descriptions.
- **The claim is "deterministic grading, no model judge," not "trace-only."**
  - Three of the four experiments use regex checks on the agent's final
    answer, and fabrication is caught only that way.
  - The paper says this plainly. RQ3 measures how well those output checks
    agree with a human reading, and that result is a finding in its own right.
- **The pilot runs on a frozen, tagged version.**
  - Hardening comes first and is checked with a few smoke runs. Smoke runs
    aren't reported as results.
  - Then a tag (v0.2.0), then the 160 sessions on that tag.
  - Each run records the git SHA, the Claude Code version and the model ID.
  - If a bug forces a fix mid-pilot, the whole affected cell is re-run on the
    new tag.
- **Chaining with the sibling tools is a design claim only.**
  - IntentLock, Taintline, Fuse, Rewind and Airlock have docs but no code yet.
  - Section 5 presents the shared core as a design. The pilot has no chained
    experiment.
- **Models:** `claude-sonnet-5-5` and `claude-haiku-5-5`, the current
  Sonnet and Haiku, set through the custom `command` template's `--model`.

## Deliverables and layout

```
agentchaos/paper/
  main.tex            acmart sigconf
  refs.bib            verified references only
  figures/            architecture and results figures
  pilot/              experiment files, raw results, analysis script (Go)
  Makefile            `make` builds main.pdf with tectonic
```

The analysis code is in Go, following the project's Go rule.

## Decisions that are the user's

- **Author list:** names, affiliation (OpenAstra?), and contact email for the
  paper.
- **Budget:** the pilot's API spend (160 sessions), and whether the user runs
  it or approves running it here.
- **Human labelling for RQ3:** who labels the transcript sample. A model can't
  stand in for "human".

## Left to the builder

- Prose, figure design, exact related-work selection (verified), table layout
  and the analysis script.
