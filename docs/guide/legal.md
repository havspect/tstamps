# Legal pages in Germany

Tstamps is software, not a legal service. This page lists what a German operator normally needs and how to put it into the app. It is not legal advice; have the texts and the processing checked by a lawyer or data protection officer.

## The two mandatory pages

A publicly reachable instance needs:

- **Impressum** — § 5 DDG (formerly § 5 TMG). Provider name, postal address, email and a second fast contact, legal form, representative, register court and number, VAT ID if you have one, and the supervisory authority if your profession requires one.
- **Datenschutzerklärung** — Art. 13 GDPR. Controller, data, purposes, legal bases, recipients, third-country transfers, retention, data subject rights and the complaint authority.

A cookie banner is **not** needed for Tstamps as shipped: there is one strictly necessary session cookie, the timeline stores its lane order in `localStorage`, and there are no analytics, advertising, external fonts or CDNs. § 25(2) TDDDG covers both. If you add analytics, a chat widget or an embedded video, that changes.

Terms and conditions (AGB) are not mandatory. Add them if you offer the service to third parties and need rules for availability, liability or termination. The BFSG accessibility rules target consumer products; an internal B2B/employee tool is normally out of scope, but check before you offer it to consumers.

## Put the pages into the instance

The server serves two operator-written HTML files and shows their links only when both exist:

```sh
sudo install -d -o tstamps -g tstamps -m 0750 /var/lib/tstamps/legal
sudo install -m 0644 imprint.html /var/lib/tstamps/legal/imprint.html
sudo install -m 0644 privacy.html /var/lib/tstamps/legal/privacy.html
```

Start `tstamps-server` with:

```sh
tstamps-server serve --addr 127.0.0.1:8080 --db /var/lib/tstamps/tstamps.db \
  --legal-dir /var/lib/tstamps/legal
```

Or set `TSTAMPS_LEGAL_DIR`. Add the flag to `ExecStart` in `deploy/tstamps-server.service`, then `systemctl daemon-reload && systemctl restart tstamps-server`. The server refuses to start if the directory is set but `imprint.html` or `privacy.html` is missing.

Links to `/imprint` and `/privacy` then appear in the homepage footer and in the app shell. The pages are read on every request, so you can correct a text without restarting. They are outside the SPA and outside the per-workspace base, because the operator of the server is the same for every workspace.

Templates live in [`deploy/legal/`](https://github.com/havspect/tstamps/tree/main/deploy/legal):

- `imprint.html.example`
- `privacy.html.example`

Copy them, fill every `[PLACEHOLDER]`, delete sections that do not apply, and check the result against what your instance actually does.

## What the privacy notice must match

The template covers the data Tstamps really stores:

| Area | Data |
| --- | --- |
| Account | Email, name, password hash, timezone, weekly hours, role, status |
| Work | Entries with start, end, timezone, project and note; plans; projects, teams, memberships |
| Security | Session token hash and 30-day expiry, API token hashes, audit events, IP and request logs |
| Signup, if enabled | Pending email address and token hash until the link is used |
| Backups | Database snapshots in `/var/backups/tstamps` and whatever you copy off the host |

Also state:

- **Hosting provider** and server location, plus the Art. 28 agreement.
- **Email provider** if self-service signup is on. Resend is US-based, so name the transfer basis or switch to an EU provider.
- **Retention**: logs, backups, accounts, entries, audit events. Pick numbers and keep them.
- **Rights and complaint authority**, and whether you have a data protection officer.
- **Processor role**: on a multi-workspace server, the customer is controller for their workspace; you process on their instruction.

## Duties beyond the two pages

- **Art. 28 GDPR agreement** with the hosting provider, and with every customer if you host for other companies.
- **Art. 30 record of processing activities** for account, time tracking, security logs and backups.
- **Art. 32 measures**: TLS, encrypted disk and backups, least privilege, patched host, short log retention, tested restore.
- **Art. 35 DPIA**: time tracking is systematic performance monitoring. Do one before employee data goes in, especially if reports or plans are used to evaluate people.
- **Works council**: § 87(1) No. 6 BetrVG gives co-determination over monitoring technology. Agree before rollout.
- **Retention**: § 16 ArbZG (working time records), §§ 147 AO and 257 HGB (payroll-relevant records). Set a deletion concept and follow it.
- **Employee notice**: employees need the Art. 13 notice at hiring; the page here is a good place, but the employment relationship may need its own wording.
- **AGPL § 13**: network users must be offered the source of the running version. The footer links to the upstream repository; if you modify Tstamps, publish your Corresponding Source too.

## The erasure gap

Tstamps v1 has no per-user erasure. Deleting an entry sets `deleted_at`, a leaver is deactivated, and audit rows stay. That is useful for restoring mistakes, but it does not satisfy an Art. 17 request on its own.

Plan a manual process before you accept employee data:

1. Take a fresh database snapshot.
2. Identify the user and the rows that carry their personal data (account, entries, plans, sessions, tokens, audit events).
3. Delete or anonymise them in a copy, verify the result, then apply it to production.
4. Let backups expire on their documented schedule.

Do not promise erasure in the privacy notice unless you can carry it out. The template says "on request, unless retention requires otherwise" for that reason.

## Multi-workspace hosting

The `/imprint` and `/privacy` pages belong to the server operator. A customer's workspace needs its own controller information and employee notice; link to it from the customer's own pages, or agree with the customer who publishes it. The operator's privacy notice should say plainly that workspace content is processed on the customer's behalf.

## Before go-live

- [ ] Impressum filled and reachable from every page
- [ ] Privacy notice filled, dated, and matching the instance
- [ ] Art. 28 agreement with the host; with customers if multi-tenant
- [ ] Art. 30 record and Art. 32 measures written down
- [ ] DPIA and works council done before employee data
- [ ] Retention periods set for logs, backups, accounts and entries
- [ ] Erasure process tested
- [ ] Source offer reachable (§ 13 AGPL)
- [ ] HTTPS, disk encryption, off-host backups
