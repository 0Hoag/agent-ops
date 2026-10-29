#!/usr/bin/env bash
set -Eeuo pipefail
[[ $# -eq 2 ]] || { printf 'usage: %s ARCHIVE TARGET_URL\n' "$0" >&2; exit 2; }
archive=$1; target_url=$2
[[ -s "$archive" ]] || { printf 'restore: archive is missing or empty: %s\n' "$archive" >&2; exit 1; }
case "$target_url" in postgres://*/*|postgresql://*/*) ;; *) printf 'restore: target must be a PostgreSQL URL with a database name\n' >&2; exit 1 ;; esac
target_db=${target_url##*/}; target_db=${target_db%%\?*}
[[ -n "$target_db" ]] || { printf 'restore: target must include a database name\n' >&2; exit 1; }
existing=$(psql "$target_url" -Atqc "SELECT count(*) FROM pg_class WHERE relkind IN ('r','p','v','m','f') AND relnamespace NOT IN (SELECT oid FROM pg_namespace WHERE nspname LIKE 'pg_%' OR nspname = 'information_schema')")
[[ "$existing" == 0 ]] || { printf 'restore: refusing to overwrite occupied target database %s (%s relations)\n' "$target_db" "$existing" >&2; exit 1; }
pg_restore --exit-on-error --no-owner --no-privileges --dbname="$target_url" "$archive"
printf 'restore: loaded %s into isolated target %s\n' "$archive" "$target_db"
