#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROJECT="agent-ops-backup-restore-$(id -u)-$$"; tmp=$(mktemp -d "${TMPDIR:-/tmp}/backup-restore.XXXXXX")
archive=/tmp/source.dump; corrupt=/tmp/corrupt.dump
compose() { docker compose -p "$PROJECT" -f "$tmp/compose.yml" "$@"; }
cleanup() { local s=$?; trap - EXIT INT TERM; compose down --volumes --remove-orphans >/dev/null 2>&1 || true; docker ps -aq --filter "label=com.docker.compose.project=$PROJECT" | xargs -r docker rm -f >/dev/null 2>&1 || true; docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT" | xargs -r docker volume rm >/dev/null 2>&1 || true; if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q . || docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then printf 'cleanup FAILED: owned resources remain\n' >&2; s=1; else printf 'cleanup verified: no owned containers or volumes (%s)\n' "$PROJECT"; fi; rm -rf "$tmp"; exit "$s"; }
trap cleanup EXIT INT TERM
cat >"$tmp/compose.yml" <<YAML
services:
  postgres:
    image: postgres:17-alpine
    environment: { POSTGRES_USER: agent_ops, POSTGRES_PASSWORD: agent_ops_dev, POSTGRES_DB: postgres }
    volumes: [pgdata:/var/lib/postgresql/data]
volumes: { pgdata: {} }
YAML
compose up -d postgres
for _ in $(seq 1 60); do compose exec -T postgres pg_isready -U agent_ops -d postgres >/dev/null 2>&1 && break; sleep 1; done
compose exec -T postgres pg_isready -U agent_ops -d postgres >/dev/null
compose exec -T postgres psql -U agent_ops -d postgres -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"${PROJECT}_source\"" -c "CREATE DATABASE \"${PROJECT}_target\""
compose exec -T postgres psql -U agent_ops -d "${PROJECT}_source" -v ON_ERROR_STOP=1 <<'SQL'
CREATE TABLE accounts (id integer PRIMARY KEY, email text NOT NULL UNIQUE);
CREATE TABLE events (id integer PRIMARY KEY, account_id integer NOT NULL REFERENCES accounts(id), kind text NOT NULL);
INSERT INTO accounts VALUES (1, 'one@example.invalid'), (2, 'two@example.invalid');
INSERT INTO events VALUES (10, 1, 'created'), (11, 2, 'created'), (12, 1, 'updated');
SQL
container=$(compose ps -q postgres)
docker cp "$ROOT/scripts/backup.sh" "$container:/tmp/backup.sh"; docker cp "$ROOT/scripts/restore.sh" "$container:/tmp/restore.sh"
docker exec "$container" chmod +x /tmp/backup.sh /tmp/restore.sh
source_url="postgresql://agent_ops:agent_ops_dev@localhost/${PROJECT}_source"; target_url="postgresql://agent_ops:agent_ops_dev@localhost/${PROJECT}_target"
docker exec "$container" /tmp/backup.sh "$source_url" "$archive"
docker exec "$container" cp "$archive" "$corrupt"
docker exec "$container" sh -c "truncate -s 13 '$corrupt'"
if docker exec "$container" /tmp/restore.sh "$corrupt" "$target_url"; then printf 'corrupt input control unexpectedly passed\n' >&2; exit 1; else printf 'corrupt input control: PASS\n'; fi
docker exec "$container" /tmp/restore.sh "$archive" "$target_url"
checksum() { compose exec -T postgres psql -U agent_ops -d "$1" -Atqc "SELECT md5(string_agg(row_text, '|' ORDER BY row_text)) FROM (SELECT 'a:'||id||':'||email row_text FROM accounts UNION ALL SELECT 'e:'||id||':'||account_id||':'||kind FROM events) q" | tr -d '[:space:]'; }
before=$(checksum "${PROJECT}_source"); after=$(checksum "${PROJECT}_target"); [[ "$before" == "$after" ]] || { printf 'checksum assertion failed: before=%s after=%s\n' "$before" "$after" >&2; exit 1; }; [[ "$before" == "$(checksum "${PROJECT}_source")" ]] || { printf 'source changed during roundtrip\n' >&2; exit 1; }; printf 'roundtrip checksum: PASS (%s)\n' "$before"
if docker exec "$container" /tmp/restore.sh "$archive" "$target_url"; then printf 'occupied target control unexpectedly passed\n' >&2; exit 1; else printf 'occupied target control: PASS\n'; fi
printf 'backup/restore fixture: PASS\n'
