# Agents

Tstamps publishes an agent skill, [`tstamps`](https://github.com/havspect/tstamps-skills), that teaches an agent to read hours, start and stop the running entry, add or correct entries, produce reports, and derive candidate entries from a calendar or from past agent work. The agent reaches the workspace through `tstamps mcp`, or through the CLI when it has a shell and the tools are unavailable.

The skill is a thin layer over the [MCP server](../mcp.md): the tools carry the permissions and the arguments, the skill carries the rules. It is one skill, not one per client, and it ships with two references the agent loads only when it needs them: deriving entries from a calendar, and from agent session history.

## Install the skill

Clone it once; the client pages assume a checkout next to your work, or copy the directory straight out of a release.

```sh
git clone https://github.com/havspect/tstamps-skills
```

Then pick the route that matches your client:

| Route | Command | Notes |
| --- | --- | --- |
| Amp | `amp skill add havspect/tstamps-skills/skills/tstamps --global` | Amp also reads the skill's `mcp.json`, so the Tstamps tools load with the skill. |
| APM | `apm install havspect/tstamps-skills#v1.0.0` | Deploys to `.agents/skills/`; pin the tag. |
| GitHub CLI | `gh skill install havspect/tstamps-skills tstamps --agent <agent> --scope user` | `<agent>` is `claude-code`, `codex`, `opencode` or `amp`. |
| Manual | copy `tstamps-skills/skills/tstamps/` into the client's skill directory | The client pages below give the path. |

MCP is configured separately in every client except Amp: `mcp.json` next to `SKILL.md` is Amp's convention, and the other clients ignore it. Each page below shows the server entry and how to verify it.

## Prerequisites

```sh
tstamps login --url https://time.example.com/w/example.com
tstamps whoami
tstamps tokens create agent
```

The token is shown once. Use a token for the agent rather than your own login, and give it to the client as `TSTAMPS_TOKEN`, or let the server read `~/.config/tstamps/config.json`. What the agent can read and write follows the token's user: a member reads only their own entries, a lead also reads their teams' projects, an admin reads everything.

## What the skill will and will not do

It proposes a candidate table — project, date, start, end, timezone, note — and writes only after you confirm. It never stops a running entry, deletes an entry or moves hours between projects unless asked.

It cannot create users, teams or projects, restore deleted entries or archived projects, change permissions, approve timesheets, handle leave or produce invoices. Those are admin-console or CLI jobs.

## Clients

- [Claude Code and the Claude desktop app](./claude.md)
- [Codex CLI, the Codex app and the IDE extension](./codex.md)
- [Langdock](./langdock.md)
- [OpenCode](./opencode.md)

Each page states what that surface cannot do, so a failed tool call is not mistaken for a broken workspace.
