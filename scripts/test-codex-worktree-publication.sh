#!/usr/bin/env bash
# Real Codex integration test; requires existing Codex authentication and jq.
set -euo pipefail

root=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/codex-worktree-publication.XXXXXX")
cleanup() {
  rm -rf "$tmp"
  test ! -e "$tmp"
  printf 'cleanup: verified absent %s\n' "$tmp"
}
trap cleanup EXIT

mkdir -p "$tmp/home" "$tmp/source/scripts"
# Reuse authentication without reading, copying or printing credentials.
ln -s "${CODEX_HOME:-$HOME/.codex}/auth.json" "$tmp/home/auth.json"
cat > "$tmp/home/worker.config.toml" <<EOF
model = "${CODEX_FIXTURE_MODEL:-gpt-reserve}"
approval_policy = "never"
sandbox_mode = "workspace-write"
project_doc_max_bytes = 0
[sandbox_workspace_write]
network_access = false
exclude_tmpdir_env_var = true
exclude_slash_tmp = true
writable_roots = ["$tmp/remote.git"]
EOF
export CODEX_HOME="$tmp/home"
git init --bare "$tmp/remote.git" >/dev/null
git init -b main "$tmp/source" >/dev/null
git -C "$tmp/source" config user.name 'Fixture Worker'
git -C "$tmp/source" config user.email fixture@example.invalid
printf 'fixture\n' > "$tmp/source/seed.txt"
git -C "$tmp/source" add seed.txt
git -C "$tmp/source" commit -m 'fixture: seed repository' >/dev/null
git -C "$tmp/source" remote add origin "$tmp/remote.git"
git -C "$tmp/source" push -u origin main >/dev/null
cp "$root/scripts/run-worker.sh" "$tmp/source/scripts/run-worker.sh"
# Keep reporting local: this fixture must not contact the application API.
printf 'report_agent_run() { :; }\n' > "$tmp/source/scripts/report-agent-run.sh"
cat > "$tmp/brief.md" <<'EOF'
This is an isolated sandbox experiment, not an application task. Execute exactly
this shell command once, with normal sandbox permissions, without escalation,
patch tools, retries or alternative Git directories. Then report its output.

printf 'published fixture\n' > publication.txt && git add publication.txt && git commit -m 'fixture: publish harmless file' && git push origin HEAD:refs/heads/fixture/worker
EOF

run_fixture() {
  if ! (cd "$tmp/source" && bash scripts/run-worker.sh fixture fixture/worker "$tmp/brief.md"); then
    cat "$events"
    return 1
  fi
  # Some native shell tools are recorded only in session response items, not
  # command_execution JSONL events. Inspect tool outputs, never model claims.
  thread=$(jq -r 'select(.type == "thread.started") | .thread_id' "$events")
  sessions=("$tmp/home/sessions"/*/*/*/*"$thread".jsonl)
  jq -c 'select(.type == "response_item") | .payload | select(.type == "custom_tool_call_output" or .type == "function_call_output") | {type, output}' "${sessions[@]}" > "$tmp/tool-output.jsonl"
  cat "$tmp/tool-output.jsonl"
}
events="$tmp/source/.agent-runs/fixture.jsonl"
worktree="$tmp/source-wt/fixture/worker"

# The unmodified launcher starts the actual installed Codex process.
run_fixture
git --git-dir="$tmp/remote.git" show-ref --verify refs/heads/fixture/worker
test "$(git --git-dir="$tmp/remote.git" show refs/heads/fixture/worker:publication.txt)" = 'published fixture'
test "$(git -C "$worktree" status --porcelain)" = ''
printf 'positive: actual Codex staged, committed and pushed publication.txt\n'

# Reset the same worktree/branch and remote; reuse the launcher's existing-worktree path.
git -C "$worktree" reset --hard origin/main >/dev/null
git -C "$worktree" update-ref -d refs/remotes/origin/fixture/worker
git --git-dir="$tmp/remote.git" update-ref -d refs/heads/fixture/worker
# Omit only the permission under test; keep config, prompt, paths and seed identical.
python3 - "$tmp/source/scripts/run-worker.sh" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
lines = path.read_text().splitlines(keepends=True)
permission = [line for line in lines if line.strip().startswith('--add-dir ')]
assert len(permission) == 2
path.write_text(''.join(line for line in lines if line not in permission))
PY
run_fixture
# Codex may exit zero after accurately reporting a failed shell command. Check
# the actual tool result and Git state, not the process exit or a fabricated error.
jq -e -s 'any(.[]; .output | tostring | test("index.lock.*(Operation not permitted|Permission denied)"))' "$tmp/tool-output.jsonl" >/dev/null
test "$(git -C "$worktree" rev-parse HEAD)" = "$(git -C "$worktree" rev-parse origin/main)"
test "$(git -C "$worktree" status --porcelain)" = '?? publication.txt'
if git --git-dir="$tmp/remote.git" show-ref --verify --quiet refs/heads/fixture/worker; then
  printf 'negative control unexpectedly published\n' >&2
  exit 1
fi
printf 'negative: actual sandbox denied git add at index.lock; no commit or remote ref\n'
