# Repository Guidelines

## Project Structure & Module Organization
- `cmd/ip/`: CLI entrypoint (`main.go`) and CLI-level tests (`main_test.go`).
- `internal/instapaper/`: Instapaper API client, types, and API tests.
- `internal/output/`: Output formatting (table/plain/json) plus golden tests in `internal/output/testdata/`.
- `internal/config/`, `internal/prompt/`, `internal/browser/`, `internal/oauth1/`, `internal/version/`: supporting modules.
- Assets/templates are not used in this repo.

## Build, Test, and Development Commands
- `go build ./cmd/ip`: Build the CLI binary in the repo root.
- `go test ./...`: Run all unit and integration tests.
- `./ip --help`: Quick local smoke test (after build) for CLI usage.

## Coding Style & Naming Conventions
- Go code is formatted with `gofmt` (tabs, standard Go style). Run `gofmt -w` on modified `.go` files.
- Names follow Go conventions: exported identifiers are `CamelCase`, unexported are `camelCase`.
- Keep CLI flags consistent with existing patterns (`--json`, `--plain`, `--output`).

## Testing Guidelines
- Framework: Go’s standard `testing` package.
- Test files follow `*_test.go` naming.
- Golden files live in `internal/output/testdata/` and should be updated carefully when output format changes.
- Run `go test ./...` before proposing changes.

## Commit & Pull Request Guidelines
- Git history isn’t available in this environment, so no verified commit convention is enforced here. Use concise, imperative commit messages (e.g., “Add progress command”).
- PRs should include a short summary, relevant commands run (build/tests), and note any behavioral changes to CLI output or exit codes.

## Security & Configuration Tips
- Never store Instapaper passwords. Use `--password-stdin` for auth and env vars for keys.
- Relevant env vars: `INSTAPAPER_CONSUMER_KEY`, `INSTAPAPER_CONSUMER_SECRET`, `INSTAPAPER_API_BASE`.
- Config lives in the user config directory (`ip config path`).
