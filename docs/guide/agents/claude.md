# Claude

Claude Code reads a skill from disk and can start `tstamps mcp` as a local stdio
server. The Claude desktop chat app can start the same server, but it does not
read a skill folder from disk: upload the skill as a ZIP.

## Prerequisites

```sh
tstamps login --url https://tstamps.example.com/w/example.com
tstamps whoami
tstamps tokens create agent     # for the agent, shown once
```

The token needs the permissions for the work you will ask for. A member token
reads and writes only its own entries; a lead token also reads its teams'
projects.

## Install the skill

Claude Code, user scope:

```sh
gh skill install havspect/tstamps-skills tstamps --agent claude-code --scope user
```

Or copy it by hand:

```sh
mkdir -p ~/.claude/skills
cp -R tstamps-skills/skills/tstamps ~/.claude/skills/
```

Project scope uses `.claude/skills/tstamps/` and is picked up by sessions in that
repository.

For the desktop chat app, package the folder and upload it:

```sh
cd tstamps-skills/skills
zip -r tstamps.zip tstamps
```

Then open **Customize > Skills**, add a skill, and upload `tstamps.zip`. The
desktop app does not read `~/.claude/skills`; a skill copied there is invisible
to the chat surface.

## Configure MCP

Claude Code, user scope:

```sh
claude mcp add --scope user \
  --env TSTAMPS_URL=https://tstamps.example.com/w/example.com \
  --env TSTAMPS_TOKEN=ts_... \
  --transport stdio tstamps -- tstamps mcp
```

If the machine already ran `tstamps login` and you want the agent to use that
config, drop both `--env` flags:

```sh
claude mcp add --scope user --transport stdio tstamps -- tstamps mcp
```

For the desktop chat app, open **Settings > Developer > Edit Config** and add
the server to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "tstamps": {
      "command": "tstamps",
      "args": ["mcp"],
      "env": {
        "TSTAMPS_URL": "https://tstamps.example.com/w/example.com",
        "TSTAMPS_TOKEN": "ts_..."
      }
    }
  }
}
```

Restart the app after saving. If the server shows as failed, give `command` the
absolute path to the binary; apps launched from the desktop do not always inherit
your shell `PATH`.

The desktop app's Code tab reads Claude Code's configuration as well, so a
server added with `claude mcp add` is available there too.

## Verify

```sh
tstamps whoami          # the account the token belongs to
claude mcp list         # tstamps should be Connected
```

In a Claude Code session run `/mcp` and confirm `tstamps` is connected, then ask:

> Call the tstamps whoami tool and tell me which workspace and user it returns.

In the desktop chat app, ask the same question in a new chat with the skill
enabled. A tool error naming the URL or token means the server started but the
credentials are wrong; no tools at all means the server did not start.

## Limits

- The desktop chat app has no shell, so the CLI fallback in the skill cannot
  run there. Only the MCP tools work.
- Skills in the desktop chat app need code execution and file creation enabled
  under **Settings > Capabilities**.
- An organization can disable local MCP servers or skill uploads on managed
  devices; if the setting is missing or the server never starts, that policy is
  the first thing to check.
- The desktop chat app stores skills in your Claude account, not on the machine.
  An uploaded skill appears on every device you sign in to, including ones
  without the `tstamps` binary.
- claude.ai in the browser cannot start a local stdio server at all. The
  uploaded skill will load, but every Tstamps tool call has nothing to reach.
  Tstamps does not ship a remote HTTP MCP endpoint; use Claude Code or the
  desktop app instead.
- The `mcp.json` next to `SKILL.md` is read by Amp only. Claude ignores it.
- Cloud sessions run elsewhere. A committed `.mcp.json` is read there, but
  `tstamps mcp` would need the binary and a login in that environment; project
  cloud threads use claude.ai connectors instead.
