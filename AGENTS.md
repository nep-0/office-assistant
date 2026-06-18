# Repository Guidelines

## Project Structure & Module Organization

This repository currently contains planning and architecture documents for a Go-first private office assistant. Treat `CONTEXT.md` as the domain glossary. Use `docs/project-scope.md`, `docs/api-design-notes.md`, `docs/implementation-roadmap.md`, `docs/evaluation-plan.md`, and `docs/risk-register.md` as the current product and engineering source of truth. ADRs live in `docs/adr/`.

As implementation begins, prefer this layout:

- `cmd/backend/`: Go API entry point.
- `internal/`: Go application packages.
- `web/`: React + TypeScript + Vite frontend.
- `deploy/` or repository root: Docker Compose and Caddy config.
- `tests/` or package-local `*_test.go`: automated tests.

## Build, Test, and Development Commands

Use standard Go commands for the backend:

- `go test ./...`: run all Go tests.
- `go fmt ./...`: format Go code.
- `go run ./cmd/backend`: run the backend locally once created.

For the frontend, use the package scripts defined in `web/package.json` once the frontend exists, such as `npm run dev`, `npm run build`, and `npm test`.

## Coding Style & Naming Conventions

Go code must be formatted with `gofmt`. Keep package names short, lowercase, and domain-oriented. Prefer clear interfaces around model providers, document processing, indexing, and storage.

Frontend code should use TypeScript, component names in `PascalCase`, hooks/utilities in `camelCase`, and compact admin-tool UI patterns described in `docs/project-scope.md`.

## Testing Guidelines

Add Go tests beside the package under test using `*_test.go`. Favor focused unit tests for provider interfaces, document status transitions, permissions, citation behavior, and indexing rules. Evaluation scripts should export CSV or JSON as described in `docs/evaluation-plan.md`.

## Commit & Pull Request Guidelines

Git history currently only shows an initial commit, so use concise imperative commit messages, for example `add provider settings model` or `implement document upload queue`.

Pull requests should include a short summary, verification steps, linked issue or task when available, screenshots for UI changes, and notes for any ADR or scope changes.

## Security & Configuration Tips

Provider API keys may be stored in SQLite for v1, but API responses must mask them. Do not log raw document content, full prompts, API keys, or full answers by default. Cloud providers are optional admin-enabled providers; the offline local path must remain demonstrable.
