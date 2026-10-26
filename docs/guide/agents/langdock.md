# Langdock

Langdock supports skills and MCP, but only remote MCP servers over Streamable
HTTP or SSE. It cannot start `tstamps mcp`, which is a local stdio process. To
use the Tstamps tools you run a small bridge on a host Langdock can reach.

## Prerequisites

- A Tstamps account with the permissions the agent needs.
- An agent token from `tstamps tokens create agent`, shown once.
- A host with the `tstamps` CLI installed, a public HTTPS name, and Node.js if
  you use the example bridge below.
- In Langdock, permission to create integrations and Member role or higher to
  create skills.

```sh
tstamps login --url https://tstamps.example.com/w/example.com
tstamps whoami
tstamps tokens create agent
```

## Install the skill

Package the whole skill directory so `references/` travels with `SKILL.md`:

```sh
cd tstamps-skills/skills
zip -r tstamps.zip tstamps
```

In Langdock open **Skills**, choose **Add Skill**, and upload `tstamps.zip`.
Uploading only `SKILL.md` also works, but the two reference files are left
behind.

Open the saved skill and attach the Tstamps integration to it, so the tools are
active whenever the skill is. Without that attachment the skill loads but cannot
call anything. The same skill can be attached to a Langdock Agent or Project;
skills enabled in regular chat do not carry over to those on their own.

## Configure MCP

Run the bridge on the host that can reach the Tstamps server. This example uses
Supergateway, a third-party stdio-to-HTTP gateway:

```sh
export TSTAMPS_URL=https://tstamps.example.com/w/example.com
export TSTAMPS_TOKEN=ts_...
export BRIDGE_KEY='a-long-random-string'

pnpm dlx supergateway \
  --stdio "tstamps mcp" \
  --outputTransport streamableHttp \
  --apiKey "$BRIDGE_KEY" \
  --host 127.0.0.1 \
  --port 8000
```

Put an HTTPS reverse proxy in front of `127.0.0.1:8000`, for example
`https://tstamps-bridge.example.com` -> `http://127.0.0.1:8000`. The MCP endpoint
is `/mcp`, so Langdock needs `https://tstamps-bridge.example.com/mcp`.

Then in Langdock:

1. Open **Integrations**, choose **Add integration > Start from scratch**, then
   **Connect remote MCP**.
2. Enter the bridge URL.
3. Choose **API Key** authentication. When the connection is created, set
   **API Key Header Type** to `Authorization: Bearer` and paste `BRIDGE_KEY`.
4. Choose **Test connection**, then **List Server Features** and enable the
   Tstamps tools you want. Save.

After saving, open each write tool and enable confirmation. New Langdock tools
do not require confirmation by default; the skill's "show the table, then act"
rule is enforced by the model, not by the platform.

## Verify

- **Test connection** and **List Server Features** should show `whoami`,
  `list_projects`, `list_entries` and the write tools.
- In a chat with the skill active, ask: "Call the tstamps whoami tool and tell
  me which workspace and user it returns."
- On the bridge host, `tstamps whoami` checks the same token independently.

`whoami` returning the token's owner while the Langdock user is someone else is
expected: the bridge has one Tstamps identity.

## Limits

- Langdock cannot run a local stdio server, and it does not read a local skill
  folder. The skill lives in Langdock; the tools come from the bridge.
- Langdock blocks private and internal addresses by default. A dedicated
  deployment can set `URL_VALIDATION_ENABLED=false`, but the server still has to
  speak Streamable HTTP or SSE.
- Every Langdock user who can use the integration acts as the single Tstamps
  account behind `TSTAMPS_TOKEN`. There is no per-user identity forwarding. Use
  a dedicated agent token, enable only the tools those users need, and do not
  describe the result as each person's own timesheet.
- Anyone who has the bridge URL and `BRIDGE_KEY` can act as that Tstamps user.
  Keep the bridge on HTTPS, keep the key in Langdock's connection settings, and
  do not expose the port directly.
- There is no shell on this surface, so the skill's CLI fallback does not run.
- The bridge must stay running. If it stops, the integration's tools fail even
  though the skill still loads.

Unverified: the exact Langdock menu labels, the ZIP layout it expects, and this
setup on a live workspace. The transport, authentication and address-validation
behavior above is from Langdock's current MCP documentation; the bridge flags are
from Supergateway's documentation.
