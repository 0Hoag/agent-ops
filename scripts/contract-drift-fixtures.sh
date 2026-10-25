#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
GATE="$ROOT/scripts/check-api-contract.sh"
CLIENT_SHA=3bedee0cff61b9f10a4fd591d8666460eba3f337
FIX="$ROOT/.agent-runs/contract-controls"
CLIENT="$FIX/client"
SRC="$ROOT/api/modules/insight/controller.go"
DOCS="$ROOT/api/docs"
if [[ -e "$FIX" ]]; then echo "Fixture path already exists: $FIX" >&2; exit 2; fi
mkdir -p "$FIX" "$FIX/api-docs"
printf '%s\n' "$PWD" > "$FIX/owner"
cp "$SRC" "$FIX/controller.go"
cp "$DOCS/docs.go" "$DOCS/swagger.json" "$DOCS/swagger.yaml" "$FIX/api-docs/"
cleanup() {
  cp "$FIX/controller.go" "$SRC"
  cp "$FIX/api-docs/docs.go" "$FIX/api-docs/swagger.json" "$FIX/api-docs/swagger.yaml" "$DOCS/"
  [[ ! -e "$FIX/api-yaml" ]] || mv "$FIX/api-yaml" "$DOCS/swagger.yaml"
  if [[ -f "$FIX/owner" ]] && [[ "$(<"$FIX/owner")" == "$PWD" ]]; then rm -rf "$FIX"; fi
}
trap cleanup EXIT

git clone -q https://github.com/0Hoag/agent-ops-client.git "$CLIENT"
git -C "$CLIENT" fetch --quiet origin "$CLIENT_SHA"
git -C "$CLIENT" checkout --quiet --detach "$CLIENT_SHA"
printf '\n<!-- tracked caller fixture -->\n' >> "$CLIENT/README.md"
printf 'untracked caller fixture\n' > "$CLIENT/.contract-untracked-fixture"

hash_inputs() {
  python3 - "$SRC" "$DOCS/docs.go" "$DOCS/swagger.json" "$DOCS/swagger.yaml" "$CLIENT/README.md" "$CLIENT/.contract-untracked-fixture" "$CLIENT/src/shared/api/swagger.json" "$CLIENT/src/shared/api/generated" <<'PY'
import hashlib, pathlib, sys
h = hashlib.sha256()
for raw in sys.argv[1:]:
    path = pathlib.Path(raw)
    h.update(raw.encode() + b'\0')
    if path.is_dir():
        for child in sorted(p for p in path.rglob('*') if p.is_file()):
            h.update(child.relative_to(path).as_posix().encode() + b'\0')
            h.update(child.read_bytes())
    elif path.is_file():
        h.update(path.read_bytes())
    else:
        h.update(b'<missing>')
print(h.hexdigest())
PY
}
expect_pass() {
  local label=$1 before after
  before=$(hash_inputs)
  if CLIENT_DIR="$CLIENT" "$GATE" >"$FIX/$label.log" 2>&1; then
    echo "PASS $label: exit 0"
  else
    cat "$FIX/$label.log"; return 1
  fi
  after=$(hash_inputs)
  [[ "$before" == "$after" ]] || { echo "FAIL $label: caller inputs changed" >&2; return 1; }
  echo "PASS $label: caller SHA256 preserved=$after"
}
expect_fail() {
  local label=$1 message=$2 rc before after
  before=$(hash_inputs)
  if CONTRACT_FIXTURE="${3:-}" CLIENT_DIR="$CLIENT" "$GATE" >"$FIX/$label.log" 2>&1; then
    echo "FAIL $label: expected exit 1"; return 1
  else
    rc=$?
  fi
  [[ $rc -eq 1 ]] || { cat "$FIX/$label.log"; echo "FAIL $label: got exit $rc"; return 1; }
  if ! grep -F "$message" "$FIX/$label.log"; then
    cat "$FIX/$label.log"; echo "FAIL $label: missing diagnostic $message"; return 1
  fi
  after=$(hash_inputs)
  [[ "$before" == "$after" ]] || { echo "FAIL $label: caller inputs changed" >&2; return 1; }
  echo "PASS $label: caller SHA256 preserved=$after"
  echo "PASS $label: exit 1; diagnostic=$message"
}

before=$(hash_inputs)
expect_pass baseline

# Add a valid, generator-recognized response annotation to a documented handler.
python3 - "$SRC" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
s = p.read_text()
needle = '// @Failure 500 {object} response.ErrorResponse'
assert needle in s
p.write_text(s.replace(needle, needle + '\n// @Failure 418 {object} response.ErrorResponse', 1))
PY
expect_fail handler "changed: api/docs/"
cp "$FIX/controller.go" "$SRC"

# The test seam removes a successfully generated API output in the disposable API copy.
expect_fail api-missing "missing: api/docs/swagger.yaml" api-missing
# Removing the baseline artifact makes the same declared, generated output new.
mv "$DOCS/swagger.yaml" "$FIX/api-yaml"
expect_fail api-new "new: api/docs/swagger.yaml"
mv "$FIX/api-yaml" "$DOCS/swagger.yaml"

printf '\n' >> "$CLIENT/src/shared/api/swagger.json"
expect_fail client-changed "changed: client/src/shared/api/swagger.json"
git -C "$CLIENT" checkout --quiet -- src/shared/api/swagger.json
# The test seam removes generated client output only after the real generator succeeds.
expect_fail client-missing "missing: client/src/shared/api/generated" client-missing
# With no caller baseline directory, the actual generated directory is an unexpected new output.
mv "$CLIENT/src/shared/api/generated" "$FIX/client-generated"
expect_fail client-new "new: client/src/shared/api/generated"
mv "$FIX/client-generated" "$CLIENT/src/shared/api/generated"

after=$(hash_inputs)
[[ "$before" == "$after" ]] || { echo "RESTORATION HASH MISMATCH before=$before after=$after" >&2; exit 1; }
echo "PASS restoration: caller/API hashes match; sha256=$after"
