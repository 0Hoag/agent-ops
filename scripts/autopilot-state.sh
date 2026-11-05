#!/usr/bin/env bash
# Small durable state/validation primitives shared by autopilot and worker dispatch.
state_root=${AUTOPILOT_STATE_DIR:-.agent-runs/recovery}

autopilot_validate_report() {
  local report=$1
  [ -s "$report" ] || { echo 'invalid coordinator report: empty' >&2; return 1; }
  IFS= read -r first < "$report" || [ -n "${first:-}" ] || { echo 'invalid coordinator report: unreadable'; return 1; }
  case "$first" in
    'AUTOPILOT: CONTINUE'|'AUTOPILOT: STOP '*) return 0 ;;
    *) echo 'invalid coordinator report: expected AUTOPILOT: CONTINUE or AUTOPILOT: STOP' >&2; return 1 ;;
  esac
}

autopilot_preflight() {
  local cmd
  for cmd in git gh jq opencode; do
    command -v "$cmd" >/dev/null 2>&1 || { echo "preflight failed: missing $cmd" >&2; return 1; }
  done
  [ -d .git ] || git rev-parse --show-toplevel >/dev/null 2>&1 || { echo 'preflight failed: not a git repository' >&2; return 1; }
  return 0
}

autopilot_reserve_atomic() {
  local file=$1 identity=$2 tmp
  [ -d "$state_root" ] || mkdir -p "$state_root"
  [ ! -e "$file" ] || return 1
  tmp="$file.$$"
  ( set -C; printf '%s\n' "$identity" > "$tmp" ) 2>/dev/null || return 1
  if ! ln "$tmp" "$file" 2>/dev/null; then rm -f "$tmp"; return 1; fi
  rm -f "$tmp"
}

autopilot_reserve_recovery() {
  local repo=$1 issue=$2 unit=$3 identity key file tmp
  [ -n "$repo" ] && [ -n "$issue" ] && [ -n "$unit" ] || { echo 'recovery requires repo, issue, and unit' >&2; return 2; }
  identity="$repo|$issue|$unit"
  key=$(printf '%s' "$identity" | shasum | awk '{print $1}')
  file="$state_root/$key"
  # A repository+issue has one recovery total; unit/wave rename cannot create another allowance.
  file="$state_root/$(printf '%s|%s' "$repo" "$issue" | shasum | awk '{print $1}')"
  if ! autopilot_reserve_atomic "$file" "$identity"; then echo "recovery refused: already reserved ($identity)" >&2; return 1; fi
  echo "recovery reserved: $identity"
}
