#!/usr/bin/env bash
set -Eeuo pipefail

# Explicit opt-in lifecycle used by verify-all. Standalone verifiers never source this
# with VERIFY_SHARED_STACK=1 and therefore retain their own ports, build, and cleanup.
shared_stack_start() {
  local root=$1 context=$2 project="agent-ops-shared-$(id -u)-$$"
  shared_stack_heavy=${VERIFY_SHARED_HEAVY:-"$root/scripts/heavy.sh"}
  local api_port=${VERIFY_SHARED_API_PORT:-$((22600 + RANDOM % 300))}
  local pg_port=${VERIFY_SHARED_PG_PORT:-$((18600 + RANDOM % 300))}
  printf 'project=%s\napi_port=%s\npg_port=%s\nroot=%s\n' "$project" "$api_port" "$pg_port" "$root" >"$context"
  export VERIFY_SHARED_STACK=1 VERIFY_SHARED_STACK_CONTEXT="$context"
  export VERIFY_SHARED_PROJECT=$project VERIFY_SHARED_API_PORT=$api_port VERIFY_SHARED_PG_PORT=$pg_port
  "$shared_stack_heavy" env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$pg_port" \
    SMOKE_API_PORT="$api_port" JWT_SECRET=verify-shared-jwt \
    REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 AGENT_RUN_TOKEN=pricing-verify-token \
    EXCHANGE_RATE_VND_PER_USD=25000 SCM_REPOSITORIES_JSON='[]' \
    docker compose -p "$project" -f "$root/docker-compose.yml" up -d --build api
  printf 'shared_stack_started project=%s api_port=%s pg_port=%s\n' "$project" "$api_port" "$pg_port" >>"$context"
}

shared_stack_cleanup() {
  local root=$1 project=$2 context=${3:-}
  shared_stack_heavy=${VERIFY_SHARED_HEAVY:-"$root/scripts/heavy.sh"}
  local api_port pg_port
  if [[ -n $context && -f $context ]]; then
    api_port=$(awk -F= '$1 == "api_port" {print $2}' "$context")
    pg_port=$(awk -F= '$1 == "pg_port" {print $2}' "$context")
  else
    api_port=${VERIFY_SHARED_API_PORT:-18080}
    pg_port=${VERIFY_SHARED_PG_PORT:-5432}
  fi
  "$shared_stack_heavy" env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$pg_port" \
    SMOKE_API_PORT="$api_port" JWT_SECRET=verify-shared-jwt \
    REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 AGENT_RUN_TOKEN=pricing-verify-token \
    EXCHANGE_RATE_VND_PER_USD=25000 SCM_REPOSITORIES_JSON='[]' \
    docker compose -p "$project" -f "$root/docker-compose.yml" \
    down --volumes --remove-orphans >/dev/null 2>&1 || return 1
  [[ -z $("$shared_stack_heavy" docker ps -aq --filter "label=com.docker.compose.project=$project") ]] || return 1
  [[ -z $("$shared_stack_heavy" docker volume ls -q --filter "label=com.docker.compose.project=$project") ]] || return 1
  [[ -z $("$shared_stack_heavy" docker network ls -q --filter "label=com.docker.compose.project=$project") ]] || return 1
}
