# Contributing

Issues and pull requests are welcome. For anything larger than a small fix, open an issue first so the approach can be agreed before you write code.

## Setup

```bash
make build      # server binary, includes UI
make test       # backend tests
make test-ui    # frontend tests
make help       # all targets
```

## Rules

Project conventions live in [CLAUDE.md](CLAUDE.md). The ones reviewers check most:

- Source files stay under 300 lines (`make filesize`).
- `make test` and `make test-ui` must finish in under 60 seconds; no `time.Sleep` or real CLI execution in tests.
- Business logic goes in `be/internal/service/`.
- No backward-compat or deprecated code paths — remove old code in the same change.
- Run `make lint` and `make deadcode` before opening a PR.

Starter tasks are labeled [`good first issue`](https://github.com/nrflo/nrflo/labels/good%20first%20issue).
