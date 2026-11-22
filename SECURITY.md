# Security Policy

## Reporting a vulnerability

Please don't report security problems in a public issue, pull request or
discussion. Use
[**Report a vulnerability**](https://github.com/sadayamuthu/agentchaos/security/advisories/new)
on the repository's Security tab instead. Only the maintainers can see the
report.

Include what you can of:

- the AgentChaos version (`agentchaos version`) or commit, your Go version and OS
- how AgentChaos was used, and its configuration with secrets removed
- steps to reproduce
- what happened and what you expected

You'll get a reply when the report has been read, and again when a fix is
ready. A fix ships as a patch release; the advisory names you unless you'd
rather it didn't.

## Supported versions

AgentChaos is at the design stage and has no releases yet. Once it does,
fixes land in the latest release only, until 1.0.

## What counts

AgentChaos runs the agent command and MCP servers named in your experiment file, as your user, wrapped by its proxy. It writes traces and reports to the output directory you choose.

AgentChaos is a security tool, so anything that breaks one of these promises
is a security issue:

- **Faults go only where the experiment says.** The proxy alters only the calls an experiment targets; every other message passes through unchanged. A path that alters other traffic is in scope.
- **No network of its own.** AgentChaos makes no network calls itself. Your agent and servers make whatever calls they would make anyway.
- **Writes only to the output directory.** A path by which AgentChaos writes outside it is in scope.

These are by design, and not vulnerabilities:

- **Experiments run real commands.** The agent and servers in an experiment file run with your permissions, and faults can make an agent take actions it otherwise wouldn't. Run experiments against staging or harmless servers, and treat experiment files like scripts.
- Traces contain tool arguments and results verbatim. Protect them like the data they contain.

A wrong result that isn't covered above is a bug, not a vulnerability. Report
it in a normal issue; see [CONTRIBUTING.md](CONTRIBUTING.md).
