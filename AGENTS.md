# Repository Guidelines

## Project Structure & Module Organization
The entrypoint lives in `cmd/picoclaw/` (CLI, gateway, onboarding). Core logic sits in `pkg/` with subpackages for the agent loop, message bus, channels (WhatsApp, Telegram, Discord, etc.), tools, providers, storage, dashboard, and cron/heartbeat services. Built-in skills are under `skills/`, documentation in `docs/` (see `PICOCLAW.md` and `docs/WHATSAPP_SETUP.md`), and static assets in `assets/`. Use `config.example.json` as the canonical configuration template.

## Build, Test, and Development Commands
- `make deps` — update Go dependencies and run `go mod tidy`.
- `make build` — build the current-platform binary into `build/`.
- `make build-all` — build multi-platform binaries.
- `make run ARGS="gateway"` — build and run the binary with CLI args.
- `make fmt` — run `go fmt ./...` on the codebase.
- `go test ./...` — run the Go test suite (no dedicated Makefile target).
- `picoclaw onboard` — initialize the config DB (`~/.picoclaw/picoclaw.db`) and workspace.
- `picoclaw gateway` / `picoclaw agent -m "..."` — run the daemon or CLI chat.

## Coding Style & Naming Conventions
Follow standard Go formatting (tabs, `gofmt` defaults). Prefer exported identifiers in `CamelCase` and unexported identifiers in `lowerCamel`. File names are lowercase with optional underscores (e.g., `http_provider.go`, `context.go`). Keep JSON/config keys in `snake_case`, matching `config.example.json`.

## Testing Guidelines
Tests use Go’s built-in `testing` package. Name test files `*_test.go` and test functions `TestXxx`. Use `go test ./...` for full coverage; add `-cover` if you need a quick coverage snapshot.

## Commit & Pull Request Guidelines
Commit history favors short, imperative summaries with conventional prefixes such as `feat:` and `docs:` (e.g., `feat: add contacts_only toggle`). Keep commits scoped and avoid mixing refactors with feature changes. Pull requests should include: a clear description of behavior changes, testing commands run, and any config changes; attach screenshots for dashboard/UI updates when relevant.

## Configuration & Security Notes
Runtime secrets live encrypted in `~/.picoclaw/picoclaw.db`; never commit API keys. Document new channels or setup steps in `docs/` and update `config.example.json` when adding config fields.
