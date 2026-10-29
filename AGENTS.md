# Tstamps

Team time tracking in Go: one binary, SQLite, embedded Svelte timeline, REST API, CLI, MCP.

- What it does and what it does not: [docs/SPEC.md](docs/SPEC.md)
- Vocabulary: [GLOSSARY.md](GLOSSARY.md)
- Expensive-to-reverse decisions: [docs/adr](docs/adr)
- Build, test and run through the just recipes, not raw `go build`/`go run`: `just build`, `just check`, `just serve`, `just build-prod`, `just dev`.
- `just serve` takes `PORT` (default 8080): `PORT=8081 just serve --db ./tstamps.db`. The UI and the manual at `/docs` are embedded at compile time by `just build`; a binary built any other way, or before `pnpm --dir docs build`, serves a stub or 404s `/docs`.
- Frontend toolchain: pnpm only — no npm, yarn or bun; `pnpm dlx`, not `npx`. Only `pnpm-lock.yaml` is committed.
- Frontend uses webawesome as a design system.
