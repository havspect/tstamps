# OpenCode

OpenCode reads `SKILL.md` from its own skills directories and can start
`tstamps mcp` as a local stdio server. This page follows OpenCode 2.x, where MCP
servers live under `mcp.servers`.

## Prerequisites

```sh
tstamps login --url https://tstamps.example.com/w/example.com
tstamps whoami
tstamps tokens create agent     # for the agent, shown once
```

## Install the skill

```sh
gh skill install havspect/tstamps-skills tstamps --agent opencode --scope user
```

Or copy it by hand:

```sh
mkdir -p ~/.config/opencode/skills
cp -R tstamps-skills/skills/tstamps ~/.config/opencode/skills/
```

For one project, use `.opencode/skills/tstamps/` instead. OpenCode also reads
`~/.claude/skills` and `~/.agents/skills`, so an existing Claude Code or Codex
install is picked up without a second copy.

## Configure MCP

```sh
opencode mcp add tstamps --global \
  --env TSTAMPS_URL=https://tstamps.example.com/w/example.com \
  --env TSTAMPS_TOKEN=ts_... \
  -- tstamps mcp
```

Drop `--global` to write the server to the current project's config. Or edit
`opencode.json` directly:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "servers": {
      "tstamps": {
        "type": "local",
        "command": ["tstamps", "mcp"],
        "environment": {
          "TSTAMPS_URL": "https://tstamps.example.com/w/example.com",
          "TSTAMPS_TOKEN": "ts_..."
        }
      }
    }
  }
}
```

Without the `environment` block the server reads the config written by
`tstamps login`.

OpenCode 1.x used `mcp.tstamps` directly instead of `mcp.servers.tstamps`. If
`opencode mcp list` does not show the server, check the version and the shape of
your config.

## Verify

```sh
tstamps whoami
opencode mcp list       # tstamps should show as connected
```

In a session, mention `@tstamps` to load the skill, then ask:

> Call the tstamps whoami tool and tell me which workspace and user it returns.

The skill also documents a CLI fallback, and OpenCode has a shell, so
`tstamps whoami` is a valid second check when the MCP tools are missing.

## Limits

- The `mcp.json` next to `SKILL.md` is read by Amp only. OpenCode needs the
  server in `opencode.json` or added with `opencode mcp add`.
- The config shape above is 2.x. A 2.x binary reading a 1.x config will not find
  the server, and a 1.x binary will not understand `mcp.servers`.
- Duplicate skill IDs resolve by source precedence. A copy in a project's
  `.opencode/skills` wins over `~/.config/opencode/skills`, which wins over
  copies in `.agents/skills` and `.claude/skills`.
- The CLI fallback sends the machine's timezone and cannot override it. For
  work in another zone, use the MCP tools, which take `timezone`.
