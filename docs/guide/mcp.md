# MCP

Tstamps supports two MCP connection modes over the same tool registry:

1. **Remote MCP (Streamable HTTP)** at `https://time.example.com/w/<slug>/mcp`, authenticated via the workspace's built-in **OAuth 2.1** authorization server (discovery, dynamic client registration, and PKCE `S256`).
2. **Local stdio MCP (`tstamps mcp`)**, which uses the same config file and token as the CLI and talks to the HTTP API without opening the database.

Use either mode when an agent should start, stop, correct or read entries with the permissions of the user who authorized it. The shipped agent skill wraps this setup for Amp, Claude, Codex, Langdock and OpenCode: see [Agents](./agents/index.md).

## Connect over Remote MCP (OAuth 2.1)

Point an MCP client that supports Streamable HTTP and OAuth 2.1 directly at your workspace's `/mcp` endpoint:

```json
{
  "mcpServers": {
    "tstamps": {
      "url": "https://time.example.com/w/example.com/mcp"
    }
  }
}
```

When the client connects without a token:

1. `/w/<slug>/mcp` replies `401` with `WWW-Authenticate` pointing to `/.well-known/oauth-protected-resource`.
2. The client discovers the workspace's OAuth 2.1 endpoints from `/.well-known/oauth-authorization-server`, registers itself as a public client at `POST /api/v1/oauth/register`, and opens `/w/<slug>/oauth/authorize` in your browser with PKCE (`S256`).
3. You sign in (if not already signed in) and approve the requested scopes (`tstamps.read`, `tstamps.write`).
4. You can review and revoke connected OAuth clients at any time on **/tokens** in the web UI.

In local development (`just dev`), point the client at Vite on `http://localhost:5173/w/<slug>/mcp` rather than `:8080` so `/w/<slug>/oauth/authorize` opens in the dev UI.

## Connect over local stdio (`tstamps mcp`)

### Give the agent its own token

If you are not using `tstamps login` (OAuth device grant) on the same machine, create a personal token named for the agent. It is shown once.

::: code-group
```sh [Linux]
tstamps tokens create agent
```

```sh [macOS]
tstamps tokens create agent
```

```powershell [Windows]
tstamps tokens create agent
```
:::

The token acts as the user who created it. Revoke it with `tstamps tokens rm <id>` when the agent no longer needs it.

## Configure the client

Point an MCP client at the `tstamps` binary and pass the server URL and token as environment variables:

```json
{
  "mcpServers": {
    "tstamps": {
      "command": "tstamps",
      "args": ["mcp"],
      "env": {
        "TSTAMPS_URL": "https://time.example.com/w/example.com",
        "TSTAMPS_TOKEN": "ts_..."
      }
    }
  }
}
```

Put this in the MCP client's own config file. On Windows, use the full path to `tstamps.exe` if the client does not inherit `Path`, for example `%LOCALAPPDATA%\tstamps\tstamps.exe`.

Set `TSTAMPS_URL` to the workspace URL, `https://time.example.com/w/example.com`, and use a token from that workspace. A token minted elsewhere is rejected. See [Workspaces](./workspaces.md).

If `TSTAMPS_URL` and `TSTAMPS_TOKEN` are omitted, the MCP server falls back to the same rules as the CLI: flags, then environment, then `~/.config/tstamps/config.json`.

## Tools

| Tool | Needs | Does |
| --- | --- | --- |
| `whoami` | valid token | Current user, admin flag, team roles |
| `list_projects` | member | Projects to log against; `all` includes archived (admin) |
| `list_entries` | member | Entries the token may read, over a date range |
| `start_timer` | member | Start a running entry on a project |
| `stop_timer` | member | Stop the running entry |
| `add_entry` | member | Create a finished entry |
| `update_entry` | member | Move, retime, re-project, re-note or correct the timezone |
| `delete_entry` | member | Soft-delete an entry; only an admin restores it |
| `report` | member | Totals per person and project |
| `list_teams` | admin | Every team |
| `list_users` | admin | Every user |

Read tools are annotated read-only. Each tool description states the permission it needs, so an agent can explain a refusal instead of retrying.

Scopes mirror the API: a member reads their own entries, a lead also reads entries on their teams' projects, an admin reads everything.

## Timezones

Tools that create an entry take an IANA `timezone`, defaulting to the machine's zone. That zone decides the entry's local day and week. An offset such as `+02:00` is not a timezone and is rejected.

## Dates and buckets

`list_entries` and `report` take `from` and `to` as plain dates, `from` inclusive and `to` exclusive. Both default to the current ISO week. `report` accepts `by`: `week`, `day` or `none`.

## What the agent cannot do

MCP exposes logging and reading, not administration. Creating users, teams, memberships or projects from an agent is not part of v1. Do that in the [admin console](./admin.md) or the [CLI](./cli.md).
