# PostgreSQL backup and isolated restore

Use the native custom archive format with explicit PostgreSQL URLs:

```bash
scripts/backup.sh 'postgres://agent_ops:agent_ops_dev@127.0.0.1:5432/agent_ops?sslmode=disable' /tmp/agent-ops.dump
scripts/restore.sh /tmp/agent-ops.dump 'postgres://agent_ops:agent_ops_dev@127.0.0.1:5433/agent_ops_restore?sslmode=disable'
```

The backup output must not already exist. Restore requires a selected target database
and refuses it when any user relation exists, so it cannot overwrite the persistent
Compose database or an occupied target. Restore into a new empty database owned by
the caller, normally in a disposable PostgreSQL container with no published host
port. `pg_dump`, `pg_restore`, and `psql` must be installed locally.

Run the disposable roundtrip fixture under the heavy-command lock:

```bash
scripts/heavy.sh bash scripts/test-backup-restore.sh
```

The fixture owns its Compose project, volume, source data, and restore database. It
checks deterministic relational checksums, source immutability, occupied-target
refusal, corrupt-input failure, and cleanup of only resources bearing its project
label.
