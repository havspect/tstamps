# Tstamps

Team time tracking in Go: one binary, SQLite, embedded Svelte timeline, REST API, CLI, MCP.

- What it does and what it does not: [docs/SPEC.md](docs/SPEC.md)
- Vocabulary: [GLOSSARY.md](GLOSSARY.md)
- Expensive-to-reverse decisions: [docs/adr](docs/adr)
- Frontend toolchain: pnpm only — no npm, yarn or bun; `pnpm dlx`, not `npx`. Only `pnpm-lock.yaml` is committed.
- Frontend uses webawesome as a design system.
