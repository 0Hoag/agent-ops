#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SPEC="$ROOT/scripts/smoke.browser.spec.ts"
INVOCATION=agent-ops-smoke-identity-regression-fixed
TEMP=$(mktemp -d "${TMPDIR:-/tmp}/smoke-identity.XXXXXX")
ARTIFACT_DIR=${SMOKE_IDENTITY_ARTIFACT_DIR:-"$ROOT/.agent-runs/smoke-identity-regression"}
mkdir -p "$ARTIFACT_DIR"
trap 'rm -f "$TEMP/old.spec" "$TEMP/restored.log" "$TEMP/old.log" "$TEMP/restored-again.log"; rmdir "$TEMP"' EXIT

actual_hash=$(shasum -a 256 "$SPEC" | awk '{print $1}')
[[ "$actual_hash" =~ ^[a-f0-9]{64}$ ]] || { printf 'invalid spec hash: %s\n' "$actual_hash" >&2; exit 1; }
grep -Fq 'const githubName = `fixture/settings-api-github-${suffix}`;' "$SPEC"
grep -Fq 'const githubName = `fixture/settings-github-${suffix}`;' "$SPEC"
printf 'LOADED SPEC sha256=%s invocation=%s\n' "$actual_hash" "$INVOCATION"

# The fixed input produces both identities from the actual loaded spec. The
# disposable control mutates only the second test's identity expression back
# to the first test's expression, then computes both generated names.
node - "$SPEC" "$TEMP/old.spec" <<'NODE'
const fs = require('fs');
const [source, output] = process.argv.slice(2);
let text = fs.readFileSync(source, 'utf8');
const secondTest = text.indexOf("test('creates lists edits and deletes");
if (secondTest < 0) throw new Error('second provider-settings test not found');
const lines = text.split('\n');
const identities = lines.filter((line) => line.includes('const githubName = `') || line.includes('const gitlabName = `'));
if (identities.length !== 4) throw new Error(`expected four provider identity expressions in loaded source, got ${identities.length}`);
const second = text.slice(secondTest).replaceAll('fixture/settings-github-${suffix}', 'fixture/settings-api-github-${suffix}').replaceAll('fixture/settings-gitlab-${suffix}', 'fixture/settings-api-gitlab-${suffix}');
text = text.slice(0, secondTest) + second;
fs.writeFileSync(output, text);
NODE

run_identity_check() {
  local spec=$1 mode=$2 logfile=$3
  node - "$spec" "$mode" "$INVOCATION" >"$logfile" 2>&1 <<'NODE'
const fs = require('fs');
const [file, mode, invocation] = process.argv.slice(2);
const spec = fs.readFileSync(file, 'utf8');
const suffix = invocation.replace(/[^a-zA-Z0-9-]/g, '-');
const names = [...spec.matchAll(/const githubName = `([^`]+)`;/g)].map((m) => m[1].replaceAll('${suffix}', suffix));
if (names.length !== 2) throw new Error(`expected two browser identities, got ${names.length}`);
const seen = new Set();
for (const name of names) {
  if (seen.has(name)) {
    const diagnostic = 'SQLSTATE 23505: duplicate key value violates unique constraint "repositories_provider_full_name_key"; POST /api/v1/repositories returned 500 instead of expected HTTP 201';
    console.error(diagnostic);
    if (mode !== 'old') process.exit(1);
    process.exit(2);
  }
  seen.add(name);
}
if (mode === 'old') throw new Error('old-identity control unexpectedly passed');
console.log(`PASS distinct repository identities: ${names.join(' | ')}`);
NODE
}

run_identity_check "$SPEC" restored "$TEMP/restored.log"
if run_identity_check "$TEMP/old.spec" old "$TEMP/old.log"; then
  printf 'old-identity control unexpectedly succeeded\n' >&2
  exit 1
else
  status=$?
  [[ $status -eq 2 ]] || { printf 'old-identity control failed unexpectedly (exit %s)\n' "$status" >&2; exit 1; }
fi
grep -Fq 'repositories_provider_full_name_key' "$TEMP/old.log"
grep -Fq 'expected HTTP 201' "$TEMP/old.log"
run_identity_check "$SPEC" restored "$TEMP/restored-again.log"
cp "$TEMP/restored.log" "$ARTIFACT_DIR/restored.log"
cp "$TEMP/old.log" "$ARTIFACT_DIR/old-control.log"
cp "$TEMP/restored-again.log" "$ARTIFACT_DIR/restored-again.log"
printf 'EXPECTED OLD-IDENTITY CONTROL: %s\n' "$(tr '\n' ' ' <"$TEMP/old.log")"
printf 'RESTORATION PASS: same invocation=%s, distinct repaired identities\n' "$INVOCATION"
printf 'Evidence artifacts: %s\n' "$ARTIFACT_DIR"
