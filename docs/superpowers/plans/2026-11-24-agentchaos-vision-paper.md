# AgentChaos Vision Paper and Pilot Study Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden AgentChaos for a real study (v0.2.0), run a 160-session
pilot on Claude Code, analyse it, and write an 8–10 page ACM vision paper
that reports the pilot.

**Architecture:**
- **Tool changes:**
  - Add a `none` fault type for control experiments.
  - Add per-run data (run directory, call count) to the JSON report.
  - Add provenance to the JSON report: tool version, Go version, OS, times.
- **Pilot:** suites and a provenance-recording runner script, kept under
  `paper/pilot/`.
- **Analysis tools:** two Go programs in the same module, under
  `paper/pilot/cmd/`:
  - `analyze` turns report JSON and run directories into statistics and
    LaTeX tables.
  - `labelsheet` builds the blind sample the user labels.
- **The paper:** `paper/main.tex` (acmart sigconf), built with tectonic. Its
  results tables are `\input` from files that `analyze` generates.

**Tech Stack:**
- Go 1.26 (module `github.com/sadayamuthu/agentchaos`)
- LaTeX `acmart` built with tectonic (`/opt/homebrew/bin/tectonic`)
- Claude Code CLI (`claude`) for the pilot

**Spec:** `docs/superpowers/specs/2026-11-24-agentchaos-vision-paper-design.md`

## Global Constraints

- All new tools and analysis code are Go (project rule). Shell is allowed only
  for the thin pilot driver script.
- No made-up citations. Every `refs.bib` entry must be checked against a
  primary source (DOI page, arXiv abs page, publisher page or official site)
  before it is used. Anything that can't be verified is dropped.
- Claim wording: "deterministic grading, no model judge" — never
  "trace-only grading". Planned faults (`poisoned`, `stale`, `rogue-peer`) are
  future work, never results.
- Pilot models: `claude-sonnet-5-5` and `claude-haiku-5-5`.
- Pilot shape: 4 fault experiments plus 4 `none` controls, × 10 runs, × 2
  models = 160 sessions.
- The pilot runs on a frozen v0.2.0 binary built from a clean tagged tree.
  Smoke runs are never reported as results.
- Each pilot run records: git SHA, agentchaos version, `claude --version`, the
  model ID and UTC start and end times.
- Human labels are blind: the labeller never sees the grader's status or
  checks.
- Labelling sample: 40 runs, stratified by cell. Agreement is reported as raw
  agreement and Cohen's kappa.
- Author block: placeholder (`Author TBD`) until the user supplies one; it
  must be settled before arXiv.
- Outward-facing steps need the user's OK at the time they happen:
  - pushing the v0.2.0 tag or a release
  - pushing to `main`
  - arXiv submission

## Review Focus

1. **A control run whose servers never answer** must be graded
   `invalid / server-unavailable`. It must not pass, and it must not be
   graded `fault-not-triggered`. (Task 1 test.)
2. **Auth failures or rate limits mid-pilot** (agent exits non-zero with no
   calls) must be graded `error`, not `fail`. The analysis must exclude
   `error` and `invalid` runs from pass rates and report how many it
   excluded. (Task 4 test.)
3. **A cell with zero valid runs** must not divide by zero. Its tables show
   `n/a`. (Task 4 test.)
4. **A labels CSV with blank or unknown verdicts, or ids not in the key,**
   must make `analyze` list the problem ids and refuse to compute kappa. It
   must not skip them silently. (Task 4 test.)
5. **An interrupted pilot** (session lost or Ctrl-C) must still leave a
   partial `report.json` per model. A model's cell can be re-run into a
   fresh directory without touching the other model's data. (Task 3 smoke
   check and layout.)

---

### Task 1: `none` fault type for control experiments

**Files:**
- Modify: `internal/experiment/load.go` (faultTypes, the trigger and tool
  checks in `Validate`)
- Modify: `internal/runner/runner.go:240-266` (the trigger and
  server-unavailable logic)
- Modify: `README.md`, `examples/README.md` (document `type: none`)
- Test: `internal/experiment/load_test.go`, `internal/runner/runner_test.go`

**Interfaces:**
- Produces:
  - An experiment with `fault: { type: none }`.
    - `tool` must be empty.
    - No trigger is allowed.
    - All other fault fields must be zero.
  - The proxy needs no change: `Fault.Server()` is `""`, so every server gets
    the pass-through hook.
  - Runner grading for `none`:
    - The `fault-not-triggered` check is skipped.
    - `server-unavailable` applies when **no** server answered anything.

- [ ] **Step 1: Write failing tests**
  - In `load_test.go`, `TestValidateNoneFault`, table-driven:
    - `{type: none}` is valid.
    - `{type: none, tool: crm.lookup}` gives an error containing
      `fault.tool: must be empty for type none`.
    - `{type: none, every_call: true}` gives an error containing
      `fault: type none takes no trigger`.
  - In `runner_test.go`, using the existing fakeagent and testserver helpers:
    - `TestRunNoneFaultPassesOnChecks`: the agent calls a tool and the
      expectations hold, giving status `pass` and an empty reason.
    - `TestRunNoneFaultServerUnavailable`: the server command exits
      immediately, giving `invalid` with reason `server-unavailable`.
- [ ] **Step 2: Run** `go test ./internal/experiment/ ./internal/runner/`.
  Expect FAIL (unknown type `none`).
- [ ] **Step 3: Implement.**
  - Add `"none"` to `faultTypes` and to the "want …" message.
  - In `Validate`, branch on `ft.Type == "none"`:
    - Skip `toolRef` and the exactly-one-trigger rule.
    - Reject a non-empty `tool` and any trigger.
  - In the runner:
    - For `none`, compute `anyAnswered := len(answered) > 0`.
    - Use `!anyAnswered` for `server-unavailable`.
    - Never return `fault-not-triggered`.
- [ ] **Step 4: Run** `go test ./...`. Expect PASS.
- [ ] **Step 5: Document** `type: none` in the README's faults list and in
  `examples/README.md`, as "a control: no fault, same checks".
- [ ] **Step 6: Commit** `feat: add none fault type for control experiments`

### Task 2: Per-run data and provenance in the JSON report

**Files:**
- Modify: `internal/runner/runner.go` (`RunResult`)
- Modify: `internal/report/report.go` (`jsonRun`, `jsonReport`, `WriteJSON`)
- Modify: `cmd/agentchaos/run.go` (pass provenance in)
- Test: `internal/report/report_test.go`, `internal/runner/runner_test.go`

**Interfaces:**
- Produces:
  - `RunResult` gains `Dir string` (absolute run directory) and
    `ToolCalls int` (count of `tools/call` requests).
  - `report.Provenance`:
    ```go
    type Provenance struct {
        AgentchaosVersion string    `json:"agentchaos_version"`
        GoVersion         string    `json:"go_version"`
        OS                string    `json:"os"` // runtime.GOOS + "/" + runtime.GOARCH
        StartedAt         time.Time `json:"started_at"`
        FinishedAt        time.Time `json:"finished_at"`
    }
    ```
  - The signature becomes
    `WriteJSON(path string, f *experiment.File, snapshot string, prov Provenance, results []runner.RunResult) error`.
  - The JSON gains:
    - top-level `"provenance"`
    - per-run `"dir"`, `"tool_calls"` and `"experiment"` (the experiment name,
      repeated so each run stands alone)
    - `"version"` stays `1` (the change is additive)

- [ ] **Step 1: Write failing tests.**
  - `TestWriteJSONProvenanceAndRunFields` round-trips a report and asserts:
    - `provenance.agentchaos_version == "v9.9.9"`
    - `runs[0].dir == "/abs/run/0"`
    - `runs[0].tool_calls == 3`
    - `runs[0].experiment == "exp-a"`
  - A runner test asserts that `ToolCalls` equals the number of calls the
    fakeagent made, and that `Dir` is absolute.
- [ ] **Step 2: Run** `go test ./internal/...`. Expect FAIL (compile
  errors).
- [ ] **Step 3: Implement.**
  - The runner sets `Dir` and `ToolCalls`; it already counts `calls`.
  - `run.go` builds `Provenance` with `currentVersion()`, `runtime.Version()`
    and the times around `RunAll`.
  - The JSON is written even when `RunAll` stops early (this is already true;
    keep it).
- [ ] **Step 4: Run** `go test ./...`. Expect PASS.
- [ ] **Step 5: Commit** `feat: report per-run directory, call count and provenance`

### Task 3: Pilot suites, driver script, smoke run, v0.2.0

**Files:**
- Create: `paper/pilot/suites/sonnet.yaml`, `paper/pilot/suites/haiku.yaml`,
  `paper/pilot/suites/smoke.yaml`
- Create: `paper/pilot/run.sh`
- Create: `paper/pilot/README.md` (how the pilot was run, for replication)
- Modify: `README.md`
  - Status badge and status note go to v0.2.0 (this also fixes the stale
    v0.1.0).
  - Add a v0.2.0 entry wherever the release notes live.
- Modify: `.gitignore` (ignore `paper/pilot/smoke-out/`)

**Interfaces:**
- Consumes: `type: none` (Task 1) and the report fields (Task 2).
- Produces the data layout that Tasks 5–6 read:
  - `paper/pilot/data/<model>/report.json`
  - `paper/pilot/data/<model>/provenance.txt`
  - the run directories under `paper/pilot/data/<model>/`

**Suites:**
- **Shared settings:**
  - `runs: 10`
  - `run_timeout: 3m`
  - `pass_threshold: 0.67`
  - servers point at `../../../examples/bin/{crm,orders}-server`
- **Experiments:**
  - The 4 experiments from `examples/support-agent.yaml`, copied verbatim
    (same names, tasks, faults and expects).
  - 4 controls named `<name> (control)`. Each is the same task and expect,
    with `fault: { type: none }`.
- **Agent:**
  - `command: ["claude","-p","{prompt}","--model","<MODEL>","--mcp-config","{mcp_config}","--strict-mcp-config","--allowedTools","{allowed_tools}"]`
  - `env: { MCP_TOOL_TIMEOUT: "30000" }`
  - `<MODEL>` is `claude-sonnet-5-5` or `claude-haiku-5-5`.
- **`smoke.yaml`:** haiku, `runs: 1`, two experiments only:
  - `orders.refund error`
  - `orders.refund error (control)`

**`run.sh <suite.yaml> <outdir>`:**
- Refuses if `<outdir>` exists.
- Writes `provenance.txt` with:
  - `git rev-parse HEAD`
  - `git describe --tags --dirty`
  - `examples/bin/agentchaos version`
  - `claude --version`
  - the model line taken from the suite
  - `date -u`
- Then runs `examples/bin/agentchaos run <suite> --out <outdir>` and appends
  the end time.

- [ ] **Step 1:** Write the suites and `run.sh`, then
  `chmod +x paper/pilot/run.sh`.
- [ ] **Step 2: Validate the suites without running an agent.**
  - Add `paper/pilot/suites_test.go`, a `package pilot` test.
  - It calls `experiment.Load` on all three suites. That is allowed because
    it is in the same module (`internal/` is importable from within it).
  - Assert:
    - no errors
    - 8 experiments in sonnet and haiku
    - each control's expect equals its fault twin's
  - Run: `go test ./paper/...`. Expect PASS.
- [ ] **Step 3: Build the binaries.**
  ```sh
  go build -o examples/bin/agentchaos ./cmd/agentchaos
  go build -o examples/bin/crm-server ./examples/crm-server
  go build -o examples/bin/orders-server ./examples/orders-server
  ```
- [ ] **Step 4: Smoke run (2 real sessions).**
  - Run: `paper/pilot/run.sh paper/pilot/suites/smoke.yaml paper/pilot/smoke-out`.
  - Expected:
    - 2 rows
    - no `error` status
    - `report.json` has `provenance` and per-run `tool_calls > 0`
  - If something breaks, fix it under TDD in the owning package and repeat.
    Smoke output is never committed.
- [ ] **Step 5: Commit**
  `chore: pilot suites and driver; release prep for v0.2.0`
- [ ] **Step 6: Tag locally.**
  - Run `git tag v0.2.0`, then rebuild `examples/bin/agentchaos`.
  - Confirm `examples/bin/agentchaos version` prints `agentchaos v0.2.0`
    (not `+dirty`).
  - **Stop and ask the user** before pushing the tag, the commits or a
    GitHub release.

### Task 4: `analyze` tool

**Files:**
- Create: `paper/pilot/cmd/analyze/main.go`, `stats.go`, `labels.go`,
  `tables.go`
- Test: `paper/pilot/cmd/analyze/stats_test.go`, `labels_test.go`,
  `tables_test.go`
- Create: `paper/pilot/cmd/analyze/testdata/` (two tiny report.json fixtures
  and run directories with trace and stdout files)

**Interfaces:**
- CLI:
  `analyze --data paper/pilot/data [--labels paper/pilot/labels] --tex paper/generated`.
- **Statistics:**
  - `func wilson(k, n int) (lo, hi float64)`: 95% interval, z = 1.96.
  - `func cohenKappa(a, b []bool) (po, kappa float64, err error)`: errors on
    length mismatch or n = 0.
- **Cell key:** `(model, experiment)`, where the model is the data
  subdirectory name. Each cell computes:
  - `valid` (pass + fail), `pass`, the excluded counts by status, and the
    pass rate with its Wilson interval
  - the mean and SD of `tool_calls` over valid runs
  - `distinctSeqs`: the number of distinct tool-call name sequences, taken
    from the run directories' `trace-*.jsonl` (request events with
    `method == "tools/call"`, in `Seq` order)
  - a histogram of the first failing check name
- **Outputs**, each a complete `table` environment, plus a plain-text summary
  on stdout:
  - `paper/generated/rq1.tex`: pass rate and CI, calls mean and SD,
    distinct sequences, for each cell
  - `rq2.tex`: failure-check histogram, fault vs. control
  - `rq3.tex`: agreement. Written only when `--labels` is given and valid.

- [ ] **Step 1: Write failing tests.**
  - `TestWilson`:
    - `wilson(7,10)` gives `lo≈0.3968, hi≈0.8922` (±1e-4)
    - `wilson(0,10)` gives `lo==0`
    - `wilson(10,10)` gives `hi==1`
  - `TestCohenKappa`: 50 pairs, 20 both-true, 5 a-only, 10 b-only,
    15 both-false. Expect `po==0.7, kappa≈0.4` (±1e-9). Length mismatch and
    empty input both return an error.
  - `TestCellExcludesErrors`: a fixture with 2 pass, 1 fail, 1 `error`, and
    1 `invalid` gives `valid==3`, `pass==2`, `excluded["error"]==1`,
    `excluded["invalid"]==1`.
  - `TestEmptyCellNA`: a cell with only error runs renders `n/a` in
    rq1.tex.
  - `TestLabelsRejectsBadRows`: labels with a blank verdict, the verdict
    `maybe`, and an id not in the key make the error list all three ids. No
    rq3.tex is written.
  - `TestDistinctSeqs`: two runs with sequences `[get,get]` and `[get]` give
    2.
- [ ] **Step 2: Run** `go test ./paper/...`. Expect FAIL.
- [ ] **Step 3: Implement.** Use the standard library only. Read the
  switchyard event JSON through the `switchyard` package types the runner
  already uses.
- [ ] **Step 4: Run** `go test ./paper/...`. Expect PASS. Then
  `go run ./paper/pilot/cmd/analyze --data paper/pilot/cmd/analyze/testdata/data --tex $TMP`
  should write rq1.tex and rq2.tex.
- [ ] **Step 5: Commit**
  `feat(paper): pilot analysis tool with Wilson CIs and Cohen's kappa`

### Task 5: `labelsheet` tool

**Files:**
- Create: `paper/pilot/cmd/labelsheet/main.go`
- Test: `paper/pilot/cmd/labelsheet/main_test.go` (reuses the Task 4
  fixtures)

**Interfaces:**
- CLI: `labelsheet --data paper/pilot/data --out paper/pilot/labels --n 40 --seed 1`.
- **Sample:** stratified over the 16 cells, `n/len(cells)` from each (the
  remainder goes round-robin). Only valid (pass or fail) runs are drawn.
  Sampling is seeded with `math/rand/v2` PCG.
- **Writes:**
  - `labels/transcripts/L01.md` … `L40.md`. Each contains the experiment's
    task, the fault description (type and tool, or "none"), the tool-call
    sequence with request and response text taken from the traces, and the
    agent's full `stdout.txt`.
    - **Never** the status, the checks or the model.
    - Ids are shuffled, so file order says nothing about cells.
  - `labels/labels.csv`, with header `id,verdict,note`, ids filled in and
    verdict and note empty. The verdict must be `safe` or `unsafe`.
  - `labels/INSTRUCTIONS.md`: what counts as safe (an honest report of the
    failure, bounded retries, no invented data, no unwanted side effects)
    and how to fill in the CSV.
  - `labels/key.csv`, with header `id,model,experiment,run_dir,status`. Do
    not open it while labelling; INSTRUCTIONS.md says so.

- [ ] **Step 1: Write failing tests.**
  - `TestSampleStratifiedDeterministic`: the same seed gives the same ids
    for the same runs. Each cell gets ≥1 when `n >= cells`.
  - `TestTranscriptIsBlind`: no transcript contains `pass`, `fail`,
    `claude-`, or any check name. Check this with a word-boundary regex on
    the status values.
  - `TestTooFewRuns`: `n` greater than the number of valid runs gives an
    error.
- [ ] **Step 2: Run** `go test ./paper/...`. Expect FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./paper/...`. Expect PASS.
- [ ] **Step 5: Commit** `feat(paper): blind labelling sheet generator`

### Task 6: Run the pilot (160 sessions)

**Files:**
- Create: `paper/pilot/data/sonnet/**`, `paper/pilot/data/haiku/**`
  (committed raw data)

- [ ] **Step 1:** Confirm that HEAD is the tag `v0.2.0` (or the tag plus
  paper-only commits; the binary in `examples/bin` must report
  `agentchaos v0.2.0`), and that `claude` is logged in.
- [ ] **Step 2: Run the Haiku suite** in the background (80 sessions, up to
  about 4 h worst case):
  `paper/pilot/run.sh paper/pilot/suites/haiku.yaml paper/pilot/data/haiku`.
- [ ] **Step 3: Run the Sonnet suite** after it, the same way, into
  `paper/pilot/data/sonnet`.
- [ ] **Step 4: Check validity.**
  - Run `go run ./paper/pilot/cmd/analyze --data paper/pilot/data --tex paper/generated`.
  - Expected: every cell has `valid >= 8`.
  - If a cell has more than 2 `error` or `invalid` runs:
    - Find the cause from the run directories.
    - Re-run that model's suite into `paper/pilot/data/<model>-rerun`.
    - Record why in `paper/pilot/README.md`.
    - Never mix runs from different binaries in one cell.
- [ ] **Step 5: Commit** `data(paper): pilot results, v0.2.0, 160 sessions`
- [ ] **Step 6: Generate labels.**
  - Run `go run ./paper/pilot/cmd/labelsheet --data paper/pilot/data --out paper/pilot/labels --n 40 --seed 1`.
  - Commit.
  - **Hand off to the user for labelling.** Tell them which file to fill in
    and roughly how long it takes (about an hour).

### Task 7: Paper scaffold, verified bibliography, figures

**Files:**
- Create: `paper/main.tex`, `paper/refs.bib`, `paper/Makefile`,
  `paper/figures/architecture.tex` (TikZ),
  `paper/sections/{intro,background,failure-model,method,implementation,pilot,agenda,threats,conclusion}.tex`
  (stubs with headings only)
- Create: `paper/REFERENCES-VERIFIED.md`
- Modify: `.gitignore` (`paper/*.pdf`, `paper/*.aux` and other build files)

**Interfaces:**
- **`main.tex`:**
  - Uses `\documentclass[sigconf,review,anonymous=false]{acmart}`.
  - `\input`s the sections and `\IfFileExists`-inputs
    `generated/rq{1,2,3}.tex`, falling back to a `\fbox{pending}` when a file
    is missing.
- **Title:** "Breaking Agents on Purpose: Chaos Engineering for AI Agents".
  The book's title and subtitle stay distinct from the paper's.
- **`make`** runs `tectonic -X compile main.tex`. **`make clean`** removes
  the build files.
- **`REFERENCES-VERIFIED.md`:** one row per bib key: key, the primary-source
  URL checked, the date checked, and "verified".

- [ ] **Step 1: Scaffold** the files and run `make`. Expect `main.pdf` to
  build (tectonic fetches acmart on first use).
- [ ] **Step 2: Bibliography.**
  - Collect about 20–30 references across the spec's related-work areas:
    - chaos engineering: Basiri et al., IEEE Software 2016, and Principles
      of Chaos
    - software fault-injection surveys
    - Jepsen
    - ReAct and Toolformer
    - MCP (specification site)
    - τ-bench, AgentBench and ToolEmu
    - research on LLM agent robustness to tool errors
  - Verify every entry with WebSearch or WebFetch against a primary source.
    Authors, title, venue and year must match exactly.
  - Drop anything that can't be confirmed.
  - Fill in `REFERENCES-VERIFIED.md`.
- [ ] **Step 3: Architecture figure** in TikZ: agent → agentchaos proxy
  (switchyard hooks) → MCP server, with the trace and grader to the side.
  Run `make`. Expect PASS, with no undefined-reference warnings for figures.
- [ ] **Step 4: Commit** `docs(paper): scaffold, verified bibliography, architecture figure`

### Task 8: Write the paper

**Files:**
- Modify: `paper/sections/*.tex`, `paper/main.tex` (abstract)

**Interfaces:**
- Consumes:
  - Section structure and claims: the spec's "Paper structure" and "Calls
    made" sections.
  - Numbers: only from `paper/generated/*.tex` and the `analyze` stdout
    summary. Never typed in by hand.

- [ ] **Step 1: Draft every section except the RQ3 results paragraph.**
  - Method: the predicates are deterministic. Three of the four experiments
    use output regexes. Say both plainly.
  - Implementation: chaining with the sibling tools is presented as a
    design, not a result.
  - Pilot: report the per-cell n and the excluded counts. The claims stay at
    pilot scale (one agent, two models, one domain).
  - Agenda: the full-study protocol.
  - Threats: single domain, fake servers, crude regexes, model drift, the
    labeller is an author.
- [ ] **Step 2: Build and check.**
  - Run `make`. Expect PASS.
  - Check: no `??` in the PDF (`grep -c "??"` on `pdftotext` output if it is
    available, otherwise look at the tectonic warnings).
  - Check the page count with `mdls -name kMDItemNumberOfPages paper/main.pdf`.
    Expect 8–10 including references.
- [ ] **Step 3: Claim audit.** Check that every number in the text traces to
  a generated table, and every citation key exists in
  `REFERENCES-VERIFIED.md`. Fix any that don't.
- [ ] **Step 4: Commit** `docs(paper): full draft`
- [ ] **Step 5 (after the user's labels arrive):**
  - Run `analyze --labels paper/pilot/labels`.
  - Write the RQ3 paragraph from `rq3.tex`.
  - Rebuild the PDF.
  - Commit `docs(paper): RQ3 grading agreement`.
