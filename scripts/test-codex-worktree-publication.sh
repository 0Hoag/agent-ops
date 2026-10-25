#!/usr/bin/env bash
# Exercise the real worker launcher against an isolated linked worktree and local bare remote.
set -euo pipefail

root=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/codex-worktree-publication.XXXXXX")
cleanup() {
  git -C "$tmp/source" worktree remove --force "$tmp/worker" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

git init --bare "$tmp/remote.git" >/dev/null
git init -b main "$tmp/source" >/dev/null
git -C "$tmp/source" config user.name 'Fixture Worker'
git -C "$tmp/source" config user.email fixture@example.invalid
printf 'fixture\n' > "$tmp/source/seed.txt"
git -C "$tmp/source" add seed.txt
git -C "$tmp/source" commit -m 'fixture: seed repository' >/dev/null
git -C "$tmp/source" remote add origin "$tmp/remote.git"
git -C "$tmp/source" push -u origin main >/dev/null
mkdir -p "$tmp/source/scripts" "$tmp/source/.agent-runs" "$tmp/bin"
cp "$root/scripts/run-worker.sh" "$tmp/source/scripts/run-worker.sh"
cp "$root/scripts/report-agent-run.sh" "$tmp/source/scripts/report-agent-run.sh"

# Keep the Codex executable real; fake only model I/O to make the publication deterministic.
cat > "$tmp/bin/codex" <<'CODEX'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == exec ]]
shift
workspace= common_parent=
while (($#)); do
  case "$1" in
    -C) workspace=$2; shift 2 ;;
    --add-dir) common_parent=$2; shift 2 ;;
    -o) shift 2 ;;
    --profile) shift 2 ;;
    --json) shift ;;
    *) shift ;;
  esac
done
[[ -d "$workspace" ]]
if [[ -n "$common_parent" ]]; then
  [[ -w "$common_parent" ]]
else
  printf 'fatal: unable to create .git/index.lock: Permission denied\n' >&2
  exit 1
fi
printf 'published fixture\n' > "$workspace/publication.txt"
git -C "$workspace" add publication.txt
git -C "$workspace" commit -m 'fixture: publish harmless file'
git -C "$workspace" push origin HEAD:refs/heads/fixture/worker
printf '{"type":"turn.completed","usage":{"input_tokens":1}}\n'
CODEX
chmod +x "$tmp/bin/codex"
cat > "$tmp/bin/jq" <<'JQ'
#!/usr/bin/env bash
printf '{"input":1,"cached_input":0,"cache_write":0,"output":0,"reasoning":0}\n'
JQ
chmod +x "$tmp/bin/jq"

# Force the unmodified launcher to resolve the fixture root, and map its derived worktree path.
cat > "$tmp/bin/git" <<'GIT'
#!/usr/bin/env bash
if [[ "$1" == rev-parse && "$2" == --show-toplevel ]]; then
  printf '%s\n' "$FIXTURE_ROOT"
elif [[ "$1" == -C && "$3" == rev-parse && "$4" == --path-format=absolute ]]; then
  exec /usr/bin/git "$@"
else
  exec /usr/bin/git "$@"
fi
GIT
chmod +x "$tmp/bin/git"
cp "$root/scripts/run-worker.sh" "$tmp/launcher-under-test.sh"
python3 - "$tmp/launcher-under-test.sh" "$tmp" <<'PY'
from pathlib import Path
import sys

launcher = Path(sys.argv[1])
text = launcher.read_text().replace(
    'wt="$root/../$(basename "$root")-wt/$branch"', 'wt="$FIXTURE_WORKTREE"'
)
launcher.write_text(text)
PY
cat > "$tmp/brief.md" <<'EOF'
Commit and push one harmless fixture file.
EOF
export PATH="$tmp/bin:$PATH" FIXTURE_ROOT="$tmp/source" FIXTURE_WORKTREE="$tmp/worker"
git -C "$tmp/source" fetch origin main >/dev/null

# Positive publication through the launcher's actual codex exec invocation.
bash "$tmp/launcher-under-test.sh" fixture fixture/worker "$tmp/brief.md" >"$tmp/positive.log" 2>&1
git --git-dir="$tmp/remote.git" show-ref --verify refs/heads/fixture/worker
git --git-dir="$tmp/remote.git" show refs/heads/fixture/worker:publication.txt

# Recreate the exact tree and remove only the added metadata permission.
git -C "$tmp/source" worktree remove --force "$tmp/worker"
git -C "$tmp/source" branch -D fixture/worker >/dev/null
git -C "$tmp/source" fetch origin main >/dev/null
git -C "$tmp/source" worktree add "$tmp/worker" -b fixture/worker main >/dev/null
python3 - "$tmp/launcher-under-test.sh" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
path.write_text(path.read_text().replace('  --add-dir "$git_common_parent" \\\n', ''))
PY
set +e
bash "$tmp/launcher-under-test.sh" fixture fixture/worker "$tmp/brief.md" >"$tmp/negative.log" 2>&1
negative_status=$?
set -e
if [[ $negative_status -eq 0 ]] || ! grep -Eiq 'index|\.git|permission|denied' "$tmp/negative.log"; then
  printf 'negative control missed Git metadata failure (status %s):\n' "$negative_status" >&2
  cat "$tmp/negative.log" >&2
  exit 1
fi
printf 'negative control failed as expected (status %s):\n' "$negative_status"
cat "$tmp/negative.log"
printf 'fixture cleanup: trap removed temporary directory %s\n' "$tmp"
