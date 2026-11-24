# Contributing to AgentChaos

Thanks for helping. AgentChaos injects tool failures (timeouts, errors, partial and malformed results) into an AI agent's MCP tool calls through a fault-injecting proxy, and grades how the agent behaves from the call trace.

AgentChaos is at **v0, in development**: the MCP proxy, the experiment format
and the `run` command work, but formats and flags may still change. The README
says what v0 does and what is still planned, and [THESIS.md](THESIS.md) says
why. Bug reports, real-world cases and feedback on the design are all useful.

- [Ways to help](#ways-to-help)
- [Reporting a bug or a design problem](#reporting-a-bug-or-a-design-problem)
- [Development setup](#development-setup)
- [Pull requests](#pull-requests)
- [Security](#security)
- [License](#license)

## Ways to help

- **Challenge the design.** Read the README and THESIS.md, and open an issue
  if something won't work in a deployment you know, or if one of the "what
  would prove us wrong" points in THESIS.md is already true.
- **Describe a real case.** An incident, a near miss, or a system this should
  work on tells us more than any feature request.
- **Pick up an area** from the "Wanted" list in
  [MAINTAINERS.md](MAINTAINERS.md), or an issue labelled
  [`good first issue`](https://github.com/sadayamuthu/agentchaos/labels/good%20first%20issue)
  or [`help wanted`](https://github.com/sadayamuthu/agentchaos/labels/help%20wanted).

For anything larger than a small fix, open an issue first so we can agree on
the approach before you write the code.

## Reporting a bug or a design problem

Please include:

1. The version (`agentchaos version`) or commit, your Go version and OS.
2. What you did, and what happened.
3. What you expected instead, and why.
4. A minimal reproduction, ideally as a failing test.

If your configuration or data is private, reduce it to made-up values that
still show the problem. Never paste credentials.

## Development setup

AgentChaos is written in **Go 1.26** or newer, like every new OpenAstra tool.

```sh
git clone https://github.com/sadayamuthu/agentchaos.git
cd agentchaos
go build ./...
```

AgentChaos is built on [switchyard](https://github.com/sadayamuthu/switchyard),
fetched as a normal Go module dependency (`go get` resolves it).

Before pushing, run what CI will run:

```sh
go test -race ./...
go vet ./...
gofmt -l .
```

`gofmt -l .` must print nothing.

**Every behaviour change needs a test.** For a bug fix, add a test that fails
before your change and passes after it.

## Pull requests

- Keep each PR to one change; small PRs get reviewed faster.
- Use the commit message style in the history: `type: summary`, for example
  `fix: handle an empty tool list`. A scope is optional (`fix(proxy): ...`).
  Types in use: `feat`, `fix`, `docs`, `ci`, `chore`, `deps`.
- Update the README when you change behaviour a user can see, and
  [ROADMAP.md](ROADMAP.md) when an item is done.

## Security

Please don't report security problems in a public issue. Use
[**Report a vulnerability**](https://github.com/sadayamuthu/agentchaos/security/advisories/new)
on the repository's Security tab instead. See [SECURITY.md](SECURITY.md) for
what to include and what's in scope.

## License

AgentChaos is licensed under the [Apache License 2.0](LICENSE). By
contributing, you agree that your contributions are licensed under the same
license.
