# Ory Fosite over GORM SQLite backs the per-workspace OAuth 2.1 server

Status: accepted, 2026-10-26. Implements the OAuth 2.1 authorization server in
[ADR-0017](0017-workspace-sso-and-oauth.md) on the storage model from
[ADR-0005](0005-gorm-pure-go-sqlite.md) and
[ADR-0019](0019-workspaces-share-one-sqlite-file.md).

Each workspace's OAuth 2.1 authorization server (authorization code with PKCE
`S256`, RFC 8628 device grant, rotating refresh tokens, dynamic public-client
registration, and RFC 8707 resource indicators) is powered by Ory Fosite
(`github.com/ory/fosite`) backed by `store.Store` in the shared SQLite file.
The first-party Svelte SPA keeps using `HttpOnly` session cookies (`Session`)
and serves the browser consent (`/w/{slug}/oauth/authorize`) and device
approval (`/w/{slug}/auth/device`) views that authorize OAuth grants for remote
MCP (`/w/{slug}/mcp`), the CLI (`tstamps login`), and the mobile app.

The GORM tables (`oauth_clients`, `oauth_codes`, `oauth_access_tokens`,
`oauth_refresh_tokens`, `oauth_device_codes`) use a hybrid schema: each row
stores Fosite's serialized `fosite.Requester` state in `session_data` alongside
explicit indexed columns (`workspace_id`, `user_id`, `client_id`, `request_id`,
`scopes`, `last_used_at`, `revoked_at`). A 32-byte HMAC secret for Fosite's
token strategy is generated on first boot and persisted in SQLite so
single-binary installs stay zero-config across restarts.

Why Fosite with a hybrid schema:

- Fosite implements the subtle RFC state machines — single-use authorization
  code replay revoking issued tokens, refresh-token rotation where reusing a
  rotated token revokes the entire `request_id` family, PKCE `S256` verification,
  and RFC 8628 device polling — through Go storage interfaces without requiring
  an external service.
- Pure Fosite storage treats `session_data` as an opaque JSON blob, which cannot
  efficiently enforce `workspace_id` isolation, list or revoke a user's active
  grants on `/me/tokens`, or revoke every token in the same transaction when a
  user changes their password (`SetUserPassword`) or is deactivated (`SaveUser`).
  Promoting `workspace_id`, `user_id`, `client_id`, and `request_id` to indexed
  columns keeps those invariants fast and enforced at the `Store` boundary.

Considered and rejected: a hand-rolled OAuth 2.1 state machine (smaller
dependency footprint, but high security risk around code replay and refresh
family revocation); pure opaque `session_data` blobs without relational columns
(cannot query or cascade-revoke by `workspace_id` and `user_id` without scanning
JSON); moving the web SPA itself off `HttpOnly` session cookies onto OAuth PKCE
(adds a redirect loop to first-party login while still requiring a browser
session cookie to complete `/oauth/authorize`); running an external OAuth
daemon (breaks the single-binary, single-SQLite-file promise of ADR-0001).
