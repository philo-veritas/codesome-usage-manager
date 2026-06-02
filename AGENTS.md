# Repository Guidelines

## Project Structure & Module Organization

This is a Go module for a Codesome CLI and HTTP API. The module root is the repository root.

- `main.go` starts the CLI; `cmd/` contains Cobra commands and command tests.
- `internal/` contains application packages: `auth`, `cache`, `config`, `db`, `provider`, `repository`, `server`, and `sync`.
- `docs/` stores design and migration notes.
- `scripts/` stores local helpers, including `scripts/display_usage.sh`.
- `nginx/`, `Dockerfile`, and `docker-compose*.yml` support deployment.

## Build, Test, and Development Commands

- `make build-dev` builds the local binary as `./codesome`.
- `make test` runs `go test ./...`.
- `make check` runs tests plus `go build ./...`; use it before handoff.
- `go run .` runs the default usage view from the repository root.
- `go run . db init` initializes the local SQLite database.
- `go run . user import --file users.csv --dry-run` validates CSV user imports without writing.

## Coding Style & Naming Conventions

Use standard Go formatting and run `gofmt` on modified Go files. Keep package names short, lowercase, and aligned with existing `internal/<package>` directories. Prefer direct functions over new abstractions unless they remove real duplication. Match Cobra naming: lowercase hyphenated commands such as `import-remote-keys`, or noun-first subcommands such as `sync users`.

## Testing Guidelines

Tests use Go's standard `testing` package. Place tests beside the code under test and name files `*_test.go`. Prefer table-driven tests for command parsing, repository behavior, and sync validation. For database tests, use temporary SQLite databases, not local runtime files. Run `make test` for focused verification and `make check` for broader changes.

## Commit & Pull Request Guidelines

Recent commits follow Conventional Commits with a scope, for example `feat(user): 支持 CSV 批量导入用户` or `docs(config): 标记 api_key_ids 为 legacy`. Keep each commit focused. Pull requests should describe the change, list verification commands, and call out config, database, or deployment impact.

## Security & Configuration Tips

Do not commit local state or secrets. `config.yaml`, `.codesome_auth.json`, `.usage_cache.json`, `*.db`, and the `codesome` binary are ignored intentionally. Do not use these local files as test fixtures. Document defaults in `config.yaml.example`, and keep `codesome.base_url` aligned with the active Codesome environment.

## Agent-Specific Instructions

Make surgical changes only. Do not refactor unrelated code or remove pre-existing dead code unless asked. Before multi-file edits, state assumptions and a short verification plan. When creating branches, do not use `codex` as the branch-name prefix unless explicitly requested.
