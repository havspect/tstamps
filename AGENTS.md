# Tstamps

Team time tracking in Go: one binary, SQLite, embedded Svelte timeline, REST API, CLI, MCP.

- What it does and what it does not: [docs/SPEC.md](docs/SPEC.md)
- Vocabulary: [GLOSSARY.md](GLOSSARY.md)
- Expensive-to-reverse decisions: [docs/adr](docs/adr)
- Build, test and run through the just recipes, not raw `go build`/`go run`: `just build`, `just check`, `just serve`, `just build-prod`, `just dev`.
- `just serve` takes `PORT` (default 8080): `PORT=8081 just serve --db ./tstamps.db`. The UI and the manual at `/docs` are embedded at compile time by `just build`; a binary built any other way, or before `pnpm --dir docs build`, serves a stub or 404s `/docs`.
- Frontend toolchain: pnpm only — no npm, yarn or bun; `pnpm dlx`, not `npx`. Only `pnpm-lock.yaml` is committed.
- Frontend uses webawesome as a design system.

## Pull requests

- Write the title and body in STE-100 (Simplified Technical English). Use short sentences, one idea per sentence, and simple words. Do not use marketing language.
- Keep the body short: what changed, why, and how it was tested. Put details in the code, the docs or the commit message.
- Apply a label that names the scope before you open the PR. Use an existing label when one fits, or add a `scope:<area>` label.
- If the UI changed, attach a screenshot or a short video of the changed screen.
- Run the `ponytail-review` skill on the diff before you open the PR. Fix the findings, or list the remaining findings in the PR.
