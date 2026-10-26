# Tstamps v2 — specification

Status: draft, agreed 2026-10-24. v1 is [SPEC.md](./SPEC.md); v2 does not replace it, it adds three slices on top. Vocabulary lives in [GLOSSARY.md](../GLOSSARY.md); decisions live in [docs/adr](./adr/).

v2 turns the operator-hosted install into a paid service the Operator runs, gives workspaces enterprise identity, and adds a phone app that logs time offline. It ships as three slices, in this order, each usable on its own:

1. **Identity** — per-workspace OIDC SSO, a CLI/agent login that survives enforced SSO, remote MCP.
2. **Money & control** — Stripe trial and subscription, read-only suspension, the operator console, the audit page.
3. **Mobile** — a Flutter app with full timeline parity and an offline outbox.

The order is a dependency, not a preference: the app's login is the OAuth flow from slice 1, so building it earlier means writing its authentication twice. Self-service signup becomes operator-approved in slice 1 and reopens in slice 2, so no free workspace with SSO exists before billing.

## Non-goals for v2

SAML, SCIM or directory sync, per-user erasure, automatic workspace deletion, plan tiers, seat metering or usage billing, operator impersonation, workspace deletion from the console, mobile admin screens, push notifications and background tracking, offline merge with conflict UI, i18n, approvals, leave and holidays, rates and invoicing, Jira/Linear sync, realtime co-editing, a team-wide timeline, screenshots, custom invoicing, and multi-region hosting.

## Slice 1 — Identity

### Per-workspace SSO

SSO is OIDC only. Each workspace configures one identity provider: issuer URL, client id, client secret, scopes (default `openid email profile`). Setup is admin-only, in the workspace admin console, and includes a **Test** action that completes a real authorization before the configuration is saved as verified. Secrets are stored in the workspace's rows in the `--db` database; the Operator can read the file, as with every other secret.

- Once verified, SSO is **enforced**: password login is refused for every user except the **Break-glass admin**.
- Users are provisioned just in time on the first successful login: the `email` claim must be present and `email_verified` true. An existing user with that lowercased email is adopted, not duplicated. A new user joins with no team; admins assign teams and roles as in v1.
- The IdP proves identity only. It grants no team, role or workspace access, and removing someone from the IdP does not deactivate their User (v1's **Deactivated user** semantics still apply).
- A workspace may disable SSO only while signed in as the break-glass admin. There is no per-user "use a password instead" option.
- The login page for an SSO workspace offers one button, "Sign in with SSO", plus a small break-glass form at the same URL. There is no email-domain discovery; the workspace slug is already in the URL.

### Break-glass admin

Exactly one active admin may hold the break-glass flag per workspace, with a password and TOTP. It is set when SSO is enabled (the enabling admin becomes it), changed only by the current holder, and reset by the Operator through a narrow CLI command that writes `user.break_glass_reset` to the workspace audit log. The v1 last-active-admin rule extends to it: it cannot be deactivated or demoted by anyone but itself.

### OAuth server and remote MCP

Each workspace is its own OAuth 2.1 authorization server, under `/w/{slug}`. Discovery documents, dynamic client registration, authorization code with PKCE (S256), the device grant (RFC 8628), refresh-token rotation, and resource indicators (RFC 8707) bind every token to one workspace. There is no implicit flow and no client secret on public clients.

- Redirect URIs: `https` or loopback only. Authorization codes live 60 seconds and are single use. Access tokens live 1 hour, refresh tokens 30 days and rotate on use; reuse revokes the family. Revocation is immediate and audited.
- Scopes: `tstamps.read` and `tstamps.write`. What a token may do is still decided by the user's role; a scope can narrow a token, never widen it. Admin-only operations are not a scope; they stay role-checked.
- The first-party mobile app is a built-in public client with the custom-scheme redirect, pre-registered in every workspace, so it does not depend on dynamic registration.
- Consent is one page per client, revocable from `/me/tokens`, which now lists OAuth clients beside personal tokens.

`POST|GET /w/{slug}/mcp` serves the same tool registry as `tstamps mcp` over Streamable HTTP. Stdio MCP is unchanged and stays the path for self-hosted installs, local agents and the published skill plugin. Every remote MCP request is authenticated by an OAuth access token; a token for one workspace is refused by another.

### CLI login

`tstamps login` becomes the OAuth device grant: it prints a code, opens the workspace's approval page (or prints the URL with `--no-browser`), and stores the resulting token in the existing `0600` config file. Password login survives for API tokens only where SSO is not enforced; a workspace with SSO has no CLI password path. `tstamps mcp` needs no change.

### Signup during slice 1

`POST /api/v1/signup` still verifies the email, but the verified request no longer creates a workspace. It becomes a pending request; `tstamps-server workspace approve <email>` creates the workspace, makes the requester its first admin, and emails the link. `workspace reject <email>` deletes the request. This closes the free tier until slice 2 opens trials.

### Slice 1 is done when

A workspace admin configures an IdP, a user with no prior account signs in through it and lands on the timeline, a password login is refused, the break-glass admin can still sign in, `tstamps login` obtains a token without typing a password, and an MCP client reaches `/w/{slug}/mcp` with discovery and consent and no pasted token. A pending signup can be approved from the host.

## Slice 2 — Money and control

### Plan and trial

One flat price per workspace, unlimited users, 14-day trial. There is no plan table and no seat counting. Price, currency and tax configuration are Stripe's; the number itself is a business input, not a schema field.

The workspace stores: billing contact email (default the signup creator, editable by any admin, may be an address that is not a User), Stripe customer id, subscription id, status, trial end, current period end.

### States

| State | Entered when | Effect |
| --- | --- | --- |
| `trial` | signup approved | everything works |
| `active` | first successful payment | everything works |
| `past_due` | a payment fails | everything works, banner for admins |
| `suspended` | trial ends with no subscription, or `past_due` for 14 days | read-only |

`past_due` is a grace period, not a lockout. Entering `suspended` ends every running entry at that instant with the note `stopped on suspension` (the same rule as deactivating a user, ADR-0004), because a running entry cannot be stopped through a read-only API.

Suspension is read-only, not a lockout: everyone can sign in, read the timeline and reports, export CSV, and reach billing. Every non-GET is refused with `402 urn:tstamps:error:workspace-suspended`, except `/auth/*`, `/me`, `/billing/*`, and the Stripe webhook. Export never depends on payment state. Data stays until the Operator deletes the workspace with a command.

State is derived on request: a trial whose end has passed transitions to `suspended` once, in a transaction, lazily on the next request; a nightly sweep sends the trial-ending and payment-failed emails. Reactivating does not restart the stopped entry.

### Stripe

Stripe is the system of record for subscriptions, invoices and payment methods. Checkout starts a subscription; the Billing Portal changes cards and reads invoices; signed webhooks update state. tstamps stores no invoice, price or payment rows. Webhook delivery may arrive before the workspace is registered; the mapping is the Stripe customer id plus the workspace slug in checkout metadata, and an unknown customer is logged and ignored, not guessed.

Email keeps using the existing Resend integration, which slice 2 now requires in production: trial ending, trial ended, payment failed, workspace suspended, workspace reactivated.

### Operator console

A separate app at `/operator`, authenticated by an Operator account (argon2id password + TOTP) in the `--db` database, with its own cookie scoped to that path. It lists workspaces with state, slug, created, trial end, last activity, user/project/entry counts and backup age. It may extend a trial and suspend or reactivate a workspace, and every action writes an operator audit line.

It may not open a workspace, read entries, names or notes, impersonate anyone, or delete anything. Support access is a customer-created user. Deletion stays a CLI action. Metadata counts come from one query over the database; the console never returns content.

A workspace's record is a row in the `--db` database beside its tenant rows ([ADR-0019](./adr/0019-workspaces-share-one-sqlite-file.md)). The backup already includes it, so subscription state cannot be restored out of step with the workspaces it describes.

### Workspace admin additions

Only two, because the rest of `/admin` already answers its questions:

- **Audit log page** — the existing `GET /audit` with filters and before/after detail, at `/admin/audit`.
- **Billing panel** — state, trial end, billing contact, Checkout and Portal buttons, at `/admin/billing`. Admins only.

### Slice 2 is done when

A stranger requests signup and the Operator approves it, the workspace runs a 14-day trial, converts through Checkout, and after a failed payment still reads and exports but cannot log; the Operator sees its state and metadata, and neither the console nor any API lets the Operator read a customer's entries.

## Slice 3 — Mobile

A Flutter app for iOS and Android, in `mobile/` of this repository, using the same API and the same OAuth login as the CLI. Screens: workspace URL, login, timeline, entry editor, today/week list, reports. No admin, no audit, no operator surfaces.

- **Timeline parity, touch-first**: pinch to zoom day ↔ week, pan, long-press-drag to create, edge handles to retime with larger hit targets, drag body to move, tap to edit. Keyboard shortcuts do not exist; every action has a visible control. Overlaps are marked as on the web. The 15-minute snap stays; a long-press modifier gives 1-minute precision.
- **Reports**: the same table reduced to a phone layout (project rows, day/week columns, tap to expand people), with the same filters.
- **Offline**: the app keeps a local database and an outbox. Creating an entry, starting and stopping the running entry write locally with a client-generated `client_id` and sync when connectivity returns. The server is the authority: if it already has a running entry, the app adopts it and discards the local start; edits to the same entry resolve last-write-wins. No merge UI, no CRDT.
- **API change (additive)**: `client_id` (UUID) is accepted by `POST /entries` and `POST /timer/start`, unique per user, making retries idempotent. Absent means v1 behaviour.
- **Out of scope**: push notifications, background location or tracking, admin screens, widgets, watch apps, tablet-specific layouts.
- **Release**: TestFlight and the Play internal track for v2; store listings, privacy labels and screenshots are part of the slice, not an afterthought. CI builds iOS on a macOS runner; mobile tests and analysis run only when `mobile/` or the API contract changes, to keep CI cost sane.

### Slice 3 is done when

A user installs the app, signs in through their workspace's IdP, starts a timer with no connectivity, stops it later, and both entries appear on the web timeline after sync; starting a timer twice across phone and browser leaves exactly one running entry; the reports screen matches the web numbers for the same week.

## API additions

All under `/w/{slug}/api/v1`, additive only. Host-level signup, webhook and operator routes stay at the root.

| Method | Path | Slice |
| --- | --- | --- |
| GET | `/.well-known/oauth-authorization-server` | 1 |
| GET | `/.well-known/oauth-protected-resource` | 1 |
| POST | `/oauth/register` | 1 |
| GET/POST | `/oauth/authorize` | 1 |
| POST | `/oauth/token`, `/oauth/revoke` | 1 |
| GET/POST | `/auth/device` (start, approve) | 1 |
| POST | `/auth/device/token` | 1 |
| GET/POST | `/auth/sso` (start) · `/auth/sso/callback` | 1 |
| GET/PUT/DELETE | `/admin/sso`, `POST /admin/sso/test` | 1 |
| POST/GET | `/mcp` | 1 |
| POST | `/entries`, `/timer/start` accept `client_id` | 3 |
| GET | `/billing` | 2 |
| POST | `/billing/checkout`, `/billing/portal` | 2 |
| POST | `/billing/webhook` (host-level, signed) | 2 |
| PATCH | `/admin/billing` (contact email) | 2 |

Host-level operator routes (`/operator/*`) are outside the workspace OpenAPI document, like signup.

## Schema and compatibility

Additive `AutoMigrate` fields only: `users.totp_secret`, `users.is_break_glass`; `workspaces_oidc`; `oauth_clients`, `oauth_codes`, `oauth_access_tokens`, `oauth_refresh_tokens`; `entries.client_id` with a per-user unique index. The `--db` database also holds `operator`, `operator_audit`, `workspace_state`, `billing`, beside the workspace's tenant rows.

A suspended workspace still answers `/w/{slug}/healthz` with its state and schema version. An older binary refuses a newer database, as in v1; every slice is therefore a one-way upgrade and ships with the pre-upgrade backup noted in `docs/guide/backup-upgrade.md`.

## Testing and definition of done

The existing suites keep their promises, plus:

- **SSO**: enforced login matrix (password refused, JIT create, adopt by email, unverified email refused, break-glass works), IdP outage, SSO disable, break-glass reset by the Operator.
- **OAuth**: discovery documents, PKCE downgrade refused, code replay refused, refresh rotation and reuse detection, cross-workspace token refused, scope narrowing, consent revoke, redirect URI validation.
- **Billing**: state table from webhook events including out-of-order and unknown customer, trial lazy transition, read-only matrix (every non-GET refused, export and billing allowed), running entry stopped exactly once on suspension.
- **Operator**: metadata only (a test asserts no content field can be returned), state actions audited, workspace state and billing in the same database and backup as the workspaces.
- **Mobile**: timeline math shared with web golden values, outbox retry idempotency, adopt-server-running-entry, last-write-wins edit, reports parity against the API.
- **Gate** (`just check`), on every push: `gofmt`, `go vet`, curated `golangci-lint`, `go test -race`; `svelte-check`, Prettier check, ESLint, Vitest; `dart format --set-exit-if-changed`, `flutter analyze --fatal-infos`, `flutter test`. Coverage is measured on changed lines (floor 80%), not the whole repo. A new behaviour without a test, or a TODO without an issue link, fails review.

## Risks the user must accept

- **Flutter is a third UI and a second timeline.** Every timeline fix lands twice, and the two disagree on gesture edge cases. The app needs a macOS CI runner, Apple's yearly fee and a review cycle that can delay a fix by days.
- **tstamps becomes an identity provider.** An OAuth authorization server is the highest-value target in the product; its security review is not optional and is not covered by v1's auth tests.
- **Solo, three slices, no date.** The realistic order is identity (6–8 weeks), money (6–8 weeks), mobile (12–16 weeks); anything that slips lands in the next slice, not in the same release.
- **The Operator can still read the database file.** "Metadata only" is a product and policy guarantee, not a cryptographic one; the privacy page must say so.
- **Remote MCP is a public surface** that agents, not humans, drive; rate limiting and abuse handling are new work.

## Open questions

Price and currency; trial reminder cadence (currently: day 10, day 14, suspension); whether `past_due` is 14 or 30 days; app store accounts and bundle identifiers; exact discovery paths against the current MCP authorization specification at implementation time; whether Stripe Tax is on from the first invoice.
