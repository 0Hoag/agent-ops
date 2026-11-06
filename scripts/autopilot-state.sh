#!/usr/bin/env bash
# Small durable state/validation primitives shared by autopilot and worker dispatch.
# Resolve shared state from this checkout, never from the caller's working directory.
autopilot_state_root() {
  local dir common
  dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd) || return 1
  common=$(git -C "$dir" rev-parse --git-common-dir) || return 1
  (cd "$dir" && cd "$common" && printf '%s/recovery\n' "$PWD")
}

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
  for cmd in git gh jq "$@" ${AUTOPILOT_REQUIRED_TOOLS:-}; do
    command -v "$cmd" >/dev/null 2>&1 || { echo "preflight failed: missing $cmd" >&2; return 1; }
  done
  if [ -n "${AUTOPILOT_PREFLIGHT_PROBE:-}" ]; then
    [ -x "$AUTOPILOT_PREFLIGHT_PROBE" ] && "$AUTOPILOT_PREFLIGHT_PROBE" || { echo 'preflight failed: environment probe' >&2; return 1; }
  fi
  return 0
}

autopilot_reserve_atomic() {
  local file=$1 identity=$2 tmp state_root
  state_root=$(dirname "$file")
  [ -d "$state_root" ] || mkdir -p "$state_root"
  tmp="$file.$$"
  ( set -C; printf '%s\n' "$identity" > "$tmp" ) 2>/dev/null || return 1
  if ! ln "$tmp" "$file" 2>/dev/null; then rm -f "$tmp"; return 1; fi
  rm -f "$tmp"
}

autopilot_reserve_recovery() {
  local repo=$1 issue=$2 unit=$3 identity file state_root
  [ -n "$repo" ] && [ -n "$issue" ] && [ -n "$unit" ] || { echo 'recovery requires repo, issue, and unit' >&2; return 2; }
  identity="$repo|$issue|$unit"
  state_root=$(autopilot_state_root) || return 2
  # A repository+issue has one recovery total; unit/wave rename cannot create another allowance.
  file="$state_root/$(printf '%s|%s' "$repo" "$issue" | shasum | awk '{print $1}')"
  if ! autopilot_reserve_atomic "$file" "$identity"; then echo "recovery refused: already reserved ($identity)" >&2; return 1; fi
  echo "recovery reserved: $identity"
}
