# Codex

Codex CLI, Codex in the ChatGPT desktop app, and the IDE extension read skills
from `.agents/skills` and share the same MCP configuration in
`~/.codex/config.toml`. A local stdio server such as `tstamps mcp` works on all
three.

## Prerequisites

```sh
tstamps login --url https://tstamps.example.com
tstamps whoami
tstamps tokens create agent     # for the agent, shown once
```

## Install the skill

```sh
gh skill install havspect/tstamps-skills tstamps --agent codex --scope user
```

Or copy it by hand to the user-wide location:

```sh
mkdir -p ~/.agents/skills
cp -R tstamps-skills/skills/tstamps ~/.agents/skills/
```

A repository can carry the skill in `.agents/skills/tstamps/`; Codex scans that
directory from the working directory up to the repository root. Restart Codex
if a new skill does not appear.

## Configure MCP

```sh
codex mcp add tstamps \
  --env TSTAMPS_URL=https://tstamps.example.com \
  --env TSTAMPS_TOKEN=ts_... \
  -- tstamps mcp
```

Without the `--env` pairs the server falls back to the config written by
`tstamps login` on this machine. The equivalent `~/.codex/config.toml` entry is:

```toml
[mcp_servers.tstamps]
command = "tstamps"
args = ["mcp"]

[mcp_servers.tstamps.env]
TSTAMPS_URL = "https://tstamps.example.com"
TSTAMPS_TOKEN = "ts_..."
```

The Codex app and the IDE extension use that same file. Configure the server
once with the CLI or by editing `config.toml`, then restart the app or select
**Restart extension** in the IDE. In the IDE you can also open the gear menu,
choose **MCP servers**, and add a STDIO server named `tstamps` with the command
`tstamps` and argument `mcp`. If that form does not offer environment variables,
set `TSTAMPS_URL` and `TSTAMPS_TOKEN` in `config.toml` instead.

A project can scope the server with `.codex/config.toml`, but only in a trusted
project.

## Verify

```sh
tstamps whoami
codex mcp list        # tstamps should be listed with its command
```

In the Codex TUI run `/mcp` to see the connected server, and `/skills` to
confirm the skill is listed. Then ask:

> Call the tstamps whoami tool and tell me which workspace and user it returns.

In the app or IDE, run `/mcp` in the composer or check the MCP server list, then
ask the same question. `tstamps whoami` in a terminal is the independent check:
if it fails, the problem is the CLI login, not Codex.

## Limits

- ChatGPT on the web and ChatGPT Work do not read `~/.codex/config.toml` or
  local skills. They use plugins and remote MCP servers; Tstamps has no remote
  MCP endpoint.
- Cloud Codex tasks may not have the local `tstamps` binary or your config.
  Treat the local stdio server as a desktop and CLI feature.
- The `mcp.json` next to `SKILL.md` is read by Amp only. Codex ignores it.
- The skill is available as a standalone skill in Codex; ChatGPT chat surfaces
  need it packaged as a plugin, which this repository does not provide.
