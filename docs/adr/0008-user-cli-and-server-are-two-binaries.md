# Two artifacts: a user CLI and a server

Commands that talk to the API and commands that own the database are different jobs on different machines, so they ship as two binaries. `tstamps` is the user CLI: `login`, `whoami`, entries, projects, teams, reports, tokens and `mcp`, all over a token in `~/.config/tstamps/config.json`. `tstamps-server` is the server artifact: `serve`, `workspace create` and `openapi`; it embeds the web UI and the SQLite file exactly as ADR-0001 describes. Neither binary is a superset of the other. MCP rides in the user CLI because it uses the same config and token, not because the CLI exists for agents.

Why: a CLI download should carry no SQLite driver, no Huma and no web assets, and building it should need no node toolchain. Workspace commands stay in the server artifact because `workspace create` opens the database directly, must work before a server process exists, and needs the host.

Considered options: one binary with a `--client` build tag (rejected: the same command would behave differently depending on how it was compiled); the server as a superset that keeps the client commands (rejected: two dispatch tables to keep in step, and "which tstamps is on this host?" becomes unanswerable); an admin CLI over HTTP (rejected: it cannot create the first admin, and it would be the user CLI with extra rights).

Consequence: releases publish `tstamps` for linux, macOS and Windows only; `tstamps-server` is built where it runs with `just build-prod`. A server host that also needs the CLI installs both, and their names differ. `openapi` moved with the server, so the web build generates its types from `tstamps-server openapi`.
