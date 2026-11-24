# Agent-prefix Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename fuse, rewind, intentlock, taintline and airlock to agentfuse, agentrewind, agentlock, agenttaint and agentbox, on GitHub, on disk and in all docs.

**Architecture:** Docs-only repos (no code yet). Rename on GitHub (redirects keep old links working), move the local folders, rewrite names and links with a scripted, reviewed replace, then fix cross-links in the other repos.

**Tech Stack:** gh CLI, git, sed/perl, grep.

**Spec:** `docs/superpowers/specs/2026-11-24-agent-prefix-rename-design.md`

## Global Constraints

- Mapping (repo → display): fuse→agentfuse (AgentFuse), rewind→agentrewind (AgentRewind), intentlock→agentlock (AgentLock), taintline→agenttaint (AgentTaint), airlock→agentbox (AgentBox).
- Do not rename switchyard, acldrift, hubscan, tripwire, threatdiff or oatc.
- Do not edit `agentchaos/paper/` (paper and pilot data).
- Private repos: commit to main and push. agentchaos and switchyard (public): branch `docs/rename-agent-prefix` and open a PR.
- Workspace root: `/Users/karthik/git-openastra`.

## Review Focus

1. Generic English words: "rewind", "fuse", "airlock" can appear as ordinary words (e.g. "rewind the session"). Replace only where they name the tool (capitalised name, link, repo path, code-span). Review every diff hunk.
2. Old names used inside other words (`intentlocks`, `fused`, `rewinding`): word-boundary match only.
3. Badge and CI URLs in READMEs (`github.com/sadayamuthu/<old>`, shields.io paths): must point at the new repo.
4. CITATION.cff `title` and `repository-code` fields: both updated.
5. Leftovers: after all tasks, a grep for the old names outside `paper/` and `.git` should show only intended generic-word uses.

---

### Task 1: Rename the five repos on GitHub and on disk

**Files:** the folders `fuse`, `rewind`, `intentlock`, `taintline`, `airlock` under the workspace root.

- [ ] **Step 1:** For each old→new: `gh repo rename <new> -R sadayamuthu/<old> --yes`.
- [ ] **Step 2:** `mv <old> <new>` and `git -C <new> remote set-url origin https://github.com/sadayamuthu/<new>.git`.
- [ ] **Step 3: Verify.** `gh api repos/sadayamuthu/<new> -q '.name+" "+.visibility'` prints `<new> private`; `git -C <new> fetch` succeeds.

### Task 2: Update names inside each renamed repo

**Files:** all tracked text files in the five renamed repos.

- [ ] **Step 1:** Replace, word-bounded: `github.com/sadayamuthu/<old>` → `.../<new>`; display names `Fuse`→`AgentFuse`, `Rewind`→`AgentRewind`, `IntentLock`→`AgentLock`, `Taintline`→`AgentTaint`, `Airlock`→`AgentBox`; lowercase tool names in code spans / commands / paths → new repo name. Also replace the *other* four tools' old names wherever they appear (the repos cross-reference each other).
- [ ] **Step 2: Review diff** (`git diff`) hunk by hunk against Review Focus 1–4; revert generic-word replacements.
- [ ] **Step 3: Verify.** `git grep -nwiE 'fuse|rewind|intentlock|taintline|airlock'` shows only intended generic uses.
- [ ] **Step 4: Commit and push** to main: `docs: rename <old> to <new>`.

### Task 3: Update cross-links in the other repos

**Files:** `agentchaos/README.md`, `agentchaos/THESIS.md`, `agentrisk/README.md`, `agentrisk/THESIS.md`, `hubscan/README.md`, `switchyard/README.md` (incl. the proxy chain line 73), `switchyard/THESIS.md`, `switchyard/MAINTAINERS.md`, `switchyard/ROADMAP.md`, `threatdiff/README.md`, `threatdiff/THESIS.md`.

- [ ] **Step 1:** Same replacements as Task 2. agentchaos in worktree `.agentchaos-rename` (branch `docs/rename-agent-prefix`, already holds the spec and this plan); switchyard on a new branch of the same name.
- [ ] **Step 2: Review diff** as in Task 2.
- [ ] **Step 3: Verify** with the Task 2 grep (agentchaos: exclude `paper/`).
- [ ] **Step 4: Commit.** agentrisk, hubscan, threatdiff: commit and push to main. agentchaos, switchyard: push the branch and `gh pr create`.
- [ ] **Step 5:** Final workspace-wide grep (Review Focus 5); update the memory notes to the new names.
