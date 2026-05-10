# ttools

Go CLI developer tools.

## AI commit

Generate a commit message from currently staged files and commit them:

```bash
git add .
go run . commit
```

Dry run:

```bash
go run . commit --dry-run
```

Skip confirmation:

```bash
go run . commit --yes
```

Override provider/model from CLI:

```bash
go run . commit --provider copilot --model gpt-4.1 --yes
go run . commit --provider openai --model gpt-4.1-mini --api-key "$OPENAI_API_KEY"
```

## Configuration

Configuration is loaded with Viper.

Priority:

```text
CLI flags > environment variables > config file > defaults
```

Config file:

```text
~/.config/ttools/config.toml
```

OpenAI-compatible example:

```toml
[llm]
provider = "openai"
model = "gpt-4.1-mini"
api_key = "..."
base_url = "https://api.openai.com/v1"
```

Copilot example:

```toml
[llm]
provider = "copilot"
model = "gpt-4.1"
```

Environment variables:

```bash
TTOOLS_LLM_PROVIDER=openai
TTOOLS_LLM_MODEL=gpt-4.1-mini
TTOOLS_LLM_API_KEY=...
TTOOLS_LLM_BASE_URL=https://api.openai.com/v1
```

For Copilot, `ttools` discovers OAuth tokens from the local GitHub Copilot config or `GITHUB_TOKEN`, exchanges them for a Copilot token, then calls the Copilot chat completions endpoint.

If nothing is configured, `ttools` defaults to the Copilot provider.

If `llm.provider = "copilot"` and no model is explicitly provided, `ttools` calls Copilot `/models`, filters model-picker-enabled chat models, and chooses the lowest-cost available model. The model list is cached for 30 minutes.

## Large diffs

Commit message generation uses a two-stage LLM flow:

1. summarize each staged file change
2. generate the final Conventional Commit message from the file summaries

Large file patches are truncated before stage one. Binary files, lockfiles, and generated files are summarized from metadata only so they do not dominate the prompt.

## Development

This repository supports [mise](https://mise.jdx.dev/).

Install tools:

```bash
mise install
```

Common tasks:

```bash
mise run fmt
mise run fmt-check
mise run lint
mise run test
mise run build
mise run check
```

`mise run build` writes the CLI binary to:

```text
bin/ttools
```

`mise run build-check` only checks that all Go packages compile and does not produce artifacts.

CI runs formatting checks, `go test ./...`, `go build ./...`, and `golangci-lint` on pushes and pull requests.

This project follows TDD:

```text
Red -> Green -> Refactor
```

Run tests:

```bash
go test ./...
```
