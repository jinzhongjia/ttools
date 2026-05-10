# AGENTS.md

This file records the current development workflow and project conventions for AI agents working on `ttools`.

## Project overview

`ttools` is a Go CLI for developer tooling. The main implemented feature is:

```bash
ttools commit
```

It generates an AI commit message from Git changes and commits them.

Current behavior:

- If staged files exist, generate a commit message from staged changes.
- If no files are staged but worktree changes exist, open an interactive file picker.
- Selected files are staged using `go-git`, then the AI commit flow continues.
- If nothing is configured, the default LLM provider is GitHub Copilot.

## Core principles

### TDD is required

Development follows:

```text
Red -> Green -> Refactor
```

Rules:

- Write or update tests before implementing behavior.
- For bug fixes, first add a failing regression test.
- Keep behavior covered with focused tests in the package being changed.
- Do not call real LLM services in tests.
- Do not shell out to `git` in tests or production code.

## Implementation constraints

### Git

Use pure Go Git operations only:

- Use `github.com/go-git/go-git/v5`.
- Do not call external `git` commands via `os/exec`.
- Do not link libgit2 or other C Git libraries.
- Respect staged vs unstaged behavior carefully:
  - Untracked files are not staged changes.
  - Unstaged modifications are not staged changes.
  - If no staged files exist, use the interactive staging flow.

### LLM

Use `charm.land/fantasy` for OpenAI-compatible LLM calls.

Supported providers:

- `copilot` — default provider
- `openai` — OpenAI-compatible provider

Copilot flow:

- Discover OAuth token from local GitHub Copilot config or `GITHUB_TOKEN`.
- Exchange OAuth token for Copilot token.
- Use Copilot chat completions endpoint.
- If no model is configured, fetch `/models`, filter chat-capable model-picker-enabled models, and choose the lowest-cost model.
- Cache Copilot token and models.

### Commit message generation

Use two-stage LLM summarization:

1. Summarize each staged file diff.
2. Generate the final Conventional Commit message from file summaries.

Large diff behavior:

- Truncate large per-file patches before summarization.
- Binary files, lockfiles, and generated files should be summarized from metadata only.

### CLI UX

- CLI framework: `spf13/cobra`.
- Config: `spf13/viper`.
- Interactive stage selection: Bubble Tea.
- When generating commit messages in a terminal, stream the final commit message as model tokens arrive.
- Non-streaming generation may show a moving `Generating...` indicator.
- For non-terminal output, avoid animated output so tests and pipes stay deterministic.

## Configuration

Configuration priority:

```text
CLI flags > environment variables > config file > defaults
```

Config file:

```text
~/.config/ttools/config.toml
```

Environment variables:

```bash
TTOOLS_LLM_PROVIDER
TTOOLS_LLM_MODEL
TTOOLS_LLM_API_KEY
TTOOLS_LLM_BASE_URL
```

Default provider:

```text
copilot
```

## Tooling

This project uses `mise`.

Install tools:

```bash
mise install
```

Useful tasks:

```bash
mise run fmt
mise run fmt-check
mise run lint
mise run test
mise run build
mise run build-check
mise run check
```

Notes:

- `mise run build` writes the binary to `bin/ttools`.
- `mise run build-check` runs `go build ./...` and does not produce artifacts.
- `mise run check` runs formatting check, lint, tests, and build-check.

If mise refuses to run tasks because the config is not trusted, run:

```bash
mise trust
```

## Validation commands

Before considering work complete, run:

```bash
gofmt -w main.go cmd internal
go test ./...
golangci-lint run ./...
go build ./...
```

For binary output:

```bash
mkdir -p bin && go build -o bin/ttools .
```

## CI

GitHub Actions runs on pushes to `main`/`master` and on pull requests.

CI checks:

- Formatting via `gofmt -l main.go cmd internal`
- Tests via `go test ./...`
- Build via `go build ./...`
- Lint via `golangci-lint`

## Linting

`golangci-lint` is configured with:

- `errcheck`
- `govet`
- `ineffassign`
- `staticcheck`
- `unused`

Formatters:

- `gofmt`
- `goimports`

Fix lint findings rather than disabling linters unless there is a clear reason.

## Repository layout

Important paths:

```text
cmd/                  Cobra commands and CLI UX
internal/ai/          Two-stage AI orchestration
internal/auth/        Copilot auth/token/model handling
internal/config/      Viper config loading
internal/git/         go-git repository, diff, staging, commit logic
internal/prompt/      LLM prompt construction
internal/provider/    LLM provider factory and fantasy integration
.github/workflows/   CI
.mise.toml            Local tool/task definitions
.golangci.yml         Lint configuration
```

## Coding style

- Keep packages small and testable.
- Prefer interfaces at boundaries that need fakes in tests.
- Avoid global mutable state unless it is explicitly cached and tested.
- Return errors with context where useful.
- Preserve deterministic behavior in tests.
- Keep terminal animations disabled for non-terminal writers.

## Commit behavior caveats

- `go-git` does not automatically execute local Git hooks like native `git commit` does.
- The tool intentionally operates on staged content after interactive staging.
- Do not create empty commits.
