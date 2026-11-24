# Agent-prefix rename for runtime tools — design (short)

**Intent.** Give the OpenAstra agent runtime tools a shared `agent` prefix, matching AgentChaos and AgentRisk.

**Renames (approved 2026-11-24):**

| Old | New repo | Display name |
|---|---|---|
| fuse | agentfuse | AgentFuse |
| rewind | agentrewind | AgentRewind |
| intentlock | agentlock | AgentLock |
| taintline | agenttaint | AgentTaint |
| airlock | agentbox | AgentBox |

**Non-goals.** switchyard keeps its name (public Go module, v0.1.0 published). acldrift, hubscan, tripwire (RAG tools), threatdiff and oatc are unchanged. The AgentChaos paper and pilot data are not edited.

**Approaches.** (1) Rename only on GitHub and rely on redirects: links keep working, but docs show the old names. (2) Rename on GitHub plus update every doc and cross-link (chosen). (3) Create new repos and archive the old ones: loses history and redirects. Option 2 was chosen because it keeps history and redirects and makes the docs consistent.

**Scope.**
- Each renamed repo: `gh repo rename`, rename the local folder, update the `origin` remote, update the names and links in all docs (README, THESIS, ROADMAP, CITATION.cff, community docs).
- Cross-links in agentchaos, agentrisk, hubscan, switchyard and threatdiff.
- Private repos: commit straight to main. Public repos (agentchaos, switchyard): branch and PR.

**Builder's calls.** CamelCase display names; replace prose mentions as well as links; verify with a grep that no old name remains outside paper data.
