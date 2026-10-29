#!/usr/bin/env bash
set -Eeuo pipefail
[[ $# -eq 2 ]] || { printf 'usage: %s SOURCE_URL OUTPUT_FILE\n' "$0" >&2; exit 2; }
source_url=$1; output=$2
[[ ! -e "$output" ]] || { printf 'backup: refusing to overwrite existing output: %s\n' "$output" >&2; exit 1; }
mkdir -p "$(dirname "$output")"
pg_dump --format=custom --no-owner --no-privileges --file="$output" "$source_url"
[[ -s "$output" ]] || { printf 'backup: output is empty: %s\n' "$output" >&2; exit 1; }
printf 'backup: wrote %s\n' "$output"
