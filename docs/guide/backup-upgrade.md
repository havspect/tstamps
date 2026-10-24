# Backup and upgrade

Everything lives in one SQLite file (`tstamps.db` by default), plus its `-wal` and `-shm` companions while the server runs. A backup is a copy of that file. On a hosted server each workspace has one such file under `--workspace-dir`, so backup and restore happen per workspace.

## Back up while the server runs

`tstamps-server backup` takes a consistent snapshot with SQLite's `VACUUM INTO`. It is safe while the server is serving. It refuses a destination that already exists, so a date-stamped name never overwrites yesterday's backup.

```sh
tstamps-server backup --db /var/lib/tstamps/tstamps.db \
  "/var/backups/tstamps/tstamps-$(date -u +%Y%m%dT%H%M%SZ).db"
```

For a hosted server, point it at the workspace directory. The destination is a directory that does not exist yet, and it must be outside the workspace directory:

```sh
tstamps-server backup --workspace-dir /var/lib/tstamps/workspaces \
  "/var/backups/tstamps/tstamps-$(date -u +%Y%m%dT%H%M%SZ)"
```

You get one `<slug>.db` per workspace. The snapshot set is published only when every workspace succeeds, so a failed run leaves no partial directory to mistake for a backup. Each file is consistent on its own; the set is not one cross-workspace instant.

In both forms the destination must be writable by the user running the command. The deploy timer runs the same command nightly and deletes files older than 14 days:

```sh
# deploy/tstamps-backup.service and deploy/tstamps-backup.timer
sudo systemctl list-timers tstamps-backup.timer
sudo journalctl -u tstamps-backup.service
```

The shipped unit is written for one database. In hosted mode, replace `--db` with `--workspace-dir` and make the cleanup delete old snapshot directories instead of old `.db` files; the unit file carries a comment where to change it.

## Back up a stopped server

Stop the service, then copy the database. Include `-wal` and `-shm` if they are still present, or copy the whole directory.

```sh
sudo systemctl stop tstamps-server
sudo install -m 0600 /var/lib/tstamps/tstamps.db /var/backups/tstamps/tstamps-stopped.db
sudo systemctl start tstamps-server
```

In hosted mode, copy the whole workspace directory — every `<slug>.db`, plus any `-wal` and `-shm` files.

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

Verify with `curl -fsS http://127.0.0.1:8080/healthz` and by reading one report. Migrations run at boot, and `/healthz` reports the schema version found in the file.

On a hosted server, restore one workspace by replacing its `<slug>.db`, or replace the whole directory to roll all of them back. Verify with `curl -fsS http://127.0.0.1:8080/w/{slug}/healthz`.

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

In hosted mode the same steps apply to the whole server. Migrations run at boot only for the one-workspace database; a hosted workspace migrates on its first request after the upgrade, so an untouched workspace stays on the old schema until someone opens it. Back up every workspace first with the `--workspace-dir` form above.

## Retention

- Deleted entries keep their row with `deleted_at`; an admin restores them.
- Archived projects keep their hours and can be restored.
- A leaver is deactivated, not deleted; their entries stay visible.
- Erasing one person means erasing the workspace. There is no per-user erasure in v1.

That is why a backup matters for disaster, not for everyday mistakes: everyday mistakes are undone in the [admin console](./admin.md).
