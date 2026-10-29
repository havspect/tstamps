# Backup and upgrade

Everything lives in one SQLite database (`tstamps.db` by default, `--db` or `TSTAMPS_DB`), plus its `-wal` and `-shm` companions while the server runs. It holds every workspace. A backup is a snapshot of that one file, so one backup covers every workspace on the server.

## Back up while the server runs

`tstamps-server backup` takes a consistent snapshot with SQLite's `VACUUM INTO`. It is safe while the server is serving. It refuses a destination that already exists, so a date-stamped name never overwrites yesterday's backup.

```sh
tstamps-server backup --db /var/lib/tstamps/tstamps.db \
  "/var/backups/tstamps/tstamps-$(date -u +%Y%m%dT%H%M%SZ).db"
```

The destination is one file that does not exist yet:

- The result is a single point in time for every workspace.
- It is written mode `0600` and published only when the snapshot succeeds, so a failed run leaves no partial file to mistake for a backup.
- It refuses to run when the database holds no workspaces.

The destination must be writable by the user running the command. The deploy timer runs the same command nightly and deletes files older than 14 days:

```sh
# deploy/tstamps-backup.service and deploy/tstamps-backup.timer
sudo systemctl list-timers tstamps-backup.timer
sudo journalctl -u tstamps-backup.service
```

## Back up a stopped server

Stop the service, then copy the database. Include `-wal` and `-shm` if they are still present, or copy them together with the database.

```sh
sudo systemctl stop tstamps-server
sudo install -m 0600 /var/lib/tstamps/tstamps.db /var/backups/tstamps/tstamps-stopped.db
sudo systemctl start tstamps-server
```

## Get the backup off the box

A snapshot on the same disk is not a backup. Copy `/var/backups/tstamps` with restic, borg, rclone or whatever the host already uses. Test a restore before you need one.

The CLI config file holds a bearer token. Treat it as a credential, not as data worth backing up; each user can log in again.

## Restore

```sh
sudo systemctl stop tstamps-server
sudo mv /var/lib/tstamps/tstamps.db /var/lib/tstamps/tstamps.db.broken
sudo rm -f /var/lib/tstamps/tstamps.db-wal /var/lib/tstamps/tstamps.db-shm
sudo install -m 0600 -o tstamps -g tstamps \
  /var/backups/tstamps/tstamps-20261019T020000Z.db /var/lib/tstamps/tstamps.db
sudo systemctl start tstamps-server
```

Verify with `curl -fsS http://127.0.0.1:8080/healthz` and by reading one report. Migrations run at boot, and each workspace's `/w/{slug}/healthz` reports the schema version found in the database.

This restores every workspace together. There is no per-workspace restore or export command yet, so a server restores all-or-nothing.

Remove stale `-wal` and `-shm` files: a WAL written by a different database will confuse the restored one.

## Upgrade

1. Stop the server.
2. Take a backup.
3. Install the new `tstamps-server`.
4. Start the server.

```sh
sudo systemctl stop tstamps-server
sudo -u tstamps tstamps-server backup --db /var/lib/tstamps/tstamps.db \
  "/var/backups/tstamps/tstamps-$(date -u +%Y%m%dT%H%M%SZ).db"
sudo install -m 0755 tstamps-server /usr/local/bin/tstamps-server
sudo systemctl start tstamps-server
curl -fsS http://127.0.0.1:8080/healthz
```

`AutoMigrate` runs at boot and records the schema version in the database file. Migrations are additive. A binary refuses to start on a database written by a newer schema, so there is no silent downgrade; restore the backup to go back.

On a checkout with SSH access to the host, `just deploy <host>` does the build, snapshot, swap and health check in one step ([Server on Linux](./server.md#deploy-a-new-build)).

Migrations run once at boot for every workspace in the database, so no workspace stays on an old schema until someone opens it. The schema is version 8.

There is no migration path from the earlier per-file layout or from a schema-5 `--db` database: `workspace_id` has no column default. Nothing was deployed under either schema; recreate them rather than upgrading in place.

## Retention

- Deleted entries keep their row with `deleted_at`; an admin restores them.
- Archived projects keep their hours and can be restored.
- A leaver is deactivated, not deleted; their entries stay visible.
- Erasing one person means erasing the workspace. There is no per-user erasure in v1.

That is why a backup matters for disaster, not for everyday mistakes: everyday mistakes are undone in the [admin console](./admin.md).
