#!/usr/bin/env bash
# Exit 1 when a SQL migration can lose or reject existing data (destructive), 0 when it only adds.
#
#   scripts/ci/classify-migration.sh <file.sql>...
# Destructive: DROP, TRUNCATE, RENAME, ALTER COLUMN ... TYPE / SET NOT NULL, DELETE or UPDATE without WHERE.
# Comments are stripped first; the check is a line-level regex, so a false positive sends the PR to coordinator review.
set -euo pipefail

bad=0
for f in "$@"; do
  sql=$(sed -E 's/--.*$//' "$f" | tr '\n' ' ')
  hit=$(grep -oEi 'drop +(table|column|index|constraint|view|schema|type)|truncate|rename +(column|to)|alter +column +[a-z_"]+ +(set +data +)?type|set +not +null|delete +from +[a-z_."]+ *(;|$)|update +[a-z_."]+ +set [^;]*;' <<<"$sql" | grep -Evi ' where ' | head -1 || true)
  if [ -n "$hit" ]; then echo "DESTRUCTIVE $f: $hit"; bad=1; else echo "additive    $f"; fi
done
exit $bad
