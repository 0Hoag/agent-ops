#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
GATE="$ROOT/scripts/check-api-contract.sh"
CLIENT_SHA=ac08a2757fd6913178c0ec2968a62ce36f46ff81
mkdir -p "$ROOT/.agent-runs"
FIX=$(mktemp -d "$ROOT/.agent-runs/contract-controls.XXXXXX")
CLIENT="$FIX/client"
GATE_COPY="$FIX/scripts/check-api-contract-old.sh"
mkdir -p "$(dirname "$GATE_COPY")"
cp "$GATE" "$GATE_COPY"
cp -R "$ROOT/api" "$FIX/api"
cp "$ROOT/api/modules/insight/controller.go" "$FIX/controller.go"
cp "$ROOT/api/docs/swagger.yaml" "$FIX/swagger.yaml"
chmod +x "$GATE_COPY"
export GOCACHE="$FIX/go-cache"
export GOLANGCI_LINT_CACHE="$FIX/golangci-cache"
cleanup() {
  cp "$FIX/controller.go" "$ROOT/api/modules/insight/controller.go"
  cp "$FIX/swagger.yaml" "$ROOT/api/docs/swagger.yaml"
  rm -rf "$FIX"
}
trap cleanup EXIT

git clone -q https://github.com/0Hoag/agent-ops-client.git "$CLIENT"
git -C "$CLIENT" fetch --quiet origin "$CLIENT_SHA"
OTHER_SHA=$(git -C "$CLIENT" rev-list --all --max-count=2 | tail -1)
git -C "$CLIENT" checkout --quiet --detach "$CLIENT_SHA"

hash_inputs() {
  python3 - "$CLIENT" <<'PY'
import hashlib, pathlib, sys
h = hashlib.sha256()
path = pathlib.Path(sys.argv[1])
for child in sorted(p for p in path.rglob('*') if p.is_file() and '.git' not in p.parts):
    h.update(child.relative_to(path).as_posix().encode() + b'\0' + child.read_bytes())
print(h.hexdigest())
PY
}
git_state() {
  python3 - "$CLIENT" <<'PY'
import hashlib, os, pathlib, subprocess, sys
root = pathlib.Path(sys.argv[1])
def run(*args):
    return subprocess.check_output(args, cwd=root, text=True).strip()
def digest(data):
    return hashlib.sha256(data.encode()).hexdigest()
untracked = hashlib.sha256()
for raw in subprocess.check_output(
    ["git", "ls-files", "--others", "--exclude-standard", "-z"], cwd=root
).split(b"\0"):
    if raw:
        untracked.update(raw + b"\0" + (root / os.fsdecode(raw)).read_bytes())
print("HEAD=" + run("git", "rev-parse", "HEAD"))
print("INDEX=" + digest(run("git", "diff", "--cached", "--binary")))
print("WORKTREE=" + digest(run("git", "diff", "--binary")))
print("UNTRACKED=" + untracked.hexdigest())
PY
}
expect_pass() {
  local label=$1 before after state_before state_after
  before=$(hash_inputs)
  state_before=$(git_state)
  if CLIENT_DIR="$CLIENT" "$GATE" >"$FIX/$label.log" 2>&1; then
    echo "PASS $label: exit 0"
  else
    cat "$FIX/$label.log"; return 1
  fi
  after=$(hash_inputs)
  state_after=$(git_state)
  [[ "$before" == "$after" ]] || { echo "FAIL $label: caller inputs changed" >&2; return 1; }
  [[ "$state_before" == "$state_after" ]] || { diff <(printf '%s\n' "$state_before") <(printf '%s\n' "$state_after"); echo "FAIL $label: caller git state changed" >&2; return 1; }
  echo "PASS $label: caller SHA256 preserved=$after"
}
expect_fail() {
  local label=$1 message=$2 rc before after state_before state_after
  before=$(hash_inputs)
  state_before=$(git_state)
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
  state_after=$(git_state)
  [[ "$before" == "$after" ]] || { echo "FAIL $label: caller inputs changed" >&2; return 1; }
  [[ "$state_before" == "$state_after" ]] || { diff <(printf '%s\n' "$state_before") <(printf '%s\n' "$state_after"); echo "FAIL $label: caller git state changed" >&2; return 1; }
  echo "PASS $label: caller SHA256 preserved=$after"
  echo "PASS $label: exit 1; diagnostic=$message"
}

before=$(hash_inputs)
expect_pass baseline

git -C "$CLIENT" checkout --quiet --detach "$OTHER_SHA"
expect_pass alternate-clean-checkout
printf '\ntracked generated caller fixture\n' >> "$CLIENT/src/shared/api/swagger.json"
printf '\ntracked generated caller fixture\n' >> "$CLIENT/src/shared/api/generated/index.ts"
expect_pass dirty-generated-checkout

sed 's#compare_paths "\$client_baseline"#compare_paths "\$CLIENT_DIR"#' "$GATE" > "$GATE_COPY"
chmod +x "$GATE_COPY"
if CLIENT_DIR="$CLIENT" "$GATE_COPY" >"$FIX/old-checkout-independence.log" 2>&1; then
  echo "FAIL old-checkout-independence: expected failure"; exit 1
fi
grep -F "changed: client/src/shared/api/swagger.json" "$FIX/old-checkout-independence.log"
echo "PASS old-checkout-independence: old comparison failed at intended client artifact assertion"
cp "$GATE" "$GATE_COPY"
chmod +x "$GATE_COPY"
git -C "$CLIENT" reset --hard --quiet HEAD
git -C "$CLIENT" clean -fdq
git -C "$CLIENT" checkout --quiet --detach "$CLIENT_SHA"
git -C "$CLIENT" clean -fdq

# The test seam removes a successfully generated API output in the disposable API copy.
expect_fail handler "changed: api/docs/" handler
cp "$FIX/controller.go" "$ROOT/api/modules/insight/controller.go"
expect_fail api-missing "missing: api/docs/swagger.yaml" api-missing
expect_fail api-new "new: api/docs/swagger.yaml" api-new
cp "$FIX/swagger.yaml" "$ROOT/api/docs/swagger.yaml"
expect_fail client-changed "changed: client/src/shared/api/generated" client-changed
expect_fail client-missing "missing: client/src/shared/api/generated" client-missing
expect_fail client-new "new: client/src/shared/api/generated" client-new

after=$(hash_inputs)
[[ "$before" == "$after" ]] || { echo "RESTORATION HASH MISMATCH before=$before after=$after" >&2; exit 1; }
[[ -z "$(git -C "$CLIENT" status --porcelain)" ]] || { echo "FAIL restoration: client worktree dirty" >&2; exit 1; }
[[ "$(git -C "$CLIENT" rev-parse HEAD)" == "$CLIENT_SHA" ]] || { echo "FAIL restoration: client HEAD changed" >&2; exit 1; }
echo "PASS restoration: disposable client restored; sha256=$after"
