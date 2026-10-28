#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SPEC="$ROOT/scripts/smoke.browser.spec.ts"
INVOCATION=agent-ops-smoke-identity-regression-fixed
TEMP=$(mktemp -d "${TMPDIR:-/tmp}/smoke-identity.XXXXXX")
ARTIFACT_DIR=${SMOKE_IDENTITY_ARTIFACT_DIR:-"$ROOT/.agent-runs/smoke-identity-regression"}
mkdir -p "$ARTIFACT_DIR"
trap 'rm -rf "$TEMP"' EXIT

actual_hash=$(shasum -a 256 "$SPEC" | awk '{print $1}')
[[ "$actual_hash" =~ ^[a-f0-9]{64}$ ]] || { printf 'invalid spec hash: %s\n' "$actual_hash" >&2; exit 1; }
grep -Fq 'const githubName = `fixture/settings-api-github-${suffix}`;' "$SPEC"
grep -Fq 'const githubName = `fixture/settings-github-${suffix}`;' "$SPEC"
printf 'LOADED SPEC path=%s sha256=%s invocation=%s\n' "$SPEC" "$actual_hash" "$INVOCATION" | tee "$ARTIFACT_DIR/source.txt"

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

mutated_hash=$(shasum -a 256 "$TEMP/old.spec" | awk '{print $1}')
printf 'DISPOSABLE OLD SPEC path=%s sha256=%s\n' "$TEMP/old.spec" "$mutated_hash" | tee -a "$ARTIFACT_DIR/source.txt"

printf 'FIXTURE INCOMPLETE: source mutation is prepared at %s; no browser/API/Postgres execution occurred, so no observed HTTP or SQL evidence exists.\n' "$TEMP/old.spec" | tee -a "$ARTIFACT_DIR/source.txt" >&2
exit 1
