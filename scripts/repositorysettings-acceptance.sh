#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROJECT="agent-ops-w8d-repositorysettings-$(id -u)-$$"
PORT=${W8D_REPOSITORY_POSTGRES_PORT:-55435}
DSN="postgres://agent_ops:agent_ops_dev@127.0.0.1:${PORT}/agent_ops?sslmode=disable"
cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [[ -n "${CONTROL_ROOT:-}" && -d "$CONTROL_ROOT" ]]; then rm -rf "$CONTROL_ROOT"; fi
  JWT_SECRET=w8d-disposable-secret POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PORT" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" down --volumes --remove-orphans >/dev/null 2>&1 || true
  if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then status=1; fi
  if docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then status=1; fi
  exit "$status"
}
trap cleanup EXIT INT TERM
if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then echo "port $PORT is occupied; choose W8D_REPOSITORY_POSTGRES_PORT" >&2; exit 1; fi
compose() { JWT_SECRET=w8d-disposable-secret POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PORT" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" "$@"; }
compose up -d postgres >/dev/null
for _ in $(seq 1 60); do compose exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
compose exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null
run_test() { (cd "$ROOT/api" && W8D_REPOSITORY_ACCEPTANCE_DSN="$DSN" GOCACHE=/tmp/w8d-gocache go test -count=1 -run '^TestRepositorySettingsHTTPAcceptance$' ./modules/repositorysettings); }
run_control() { (cd "$1" && W8D_REPOSITORY_ACCEPTANCE_DSN="$DSN" GOCACHE=/tmp/w8d-gocache go test -count=1 -run '^TestRepositorySettingsHTTPAcceptance$' ./modules/repositorysettings); }
echo 'BASELINE: repositorysettings HTTP CRUD, role, tenant, ciphertext and redaction acceptance'
run_test
for control in encryption tenant role redaction; do
  CONTROL_ROOT=$(mktemp -d /tmp/w8d-repositorysettings-control.XXXXXX)
  cp -a "$ROOT/api/." "$CONTROL_ROOT/"
  case "$control" in
    encryption)
      perl -0pi -e 's/return base64\.RawStdEncoding\.EncodeToString\(g\.Seal\(nonce, nonce, \[\]byte\(value\), nil\)\), nil/return value, nil/' "$CONTROL_ROOT/pkg/crypto/secret.go"
      expected='Should not be'
      ;;
    tenant)
      perl -0pi -e 's/Where\("id = \? AND organization_id = \?", id, organizationID\)/Where("id = ?", id)/' "$CONTROL_ROOT/modules/store/repository_repositories.go"
      perl -0pi -e 's/Where\("id = \? AND organization_id = \?", id, organizationID\)/Where("id = ?", id)/' "$CONTROL_ROOT/modules/store/repository_repositories.go"
      expected='expected: 404'
      ;;
    role)
      perl -0pi -e 's/p, err := c\.orgs\.Membership/_, err := c.orgs.Membership/; s/\n\tif write && p\.Role != "owner" && p\.Role != "admin" \{\n\t\tresponse\.Forbidden\(ctx, "repository settings require owner or admin"\)\n\t\treturn "", false\n\t\}//' "$CONTROL_ROOT/modules/repositorysettings/controller.go"
      expected='expected: 403'
      ;;
    redaction)
      perl -0pi -e 's/(type View struct \{\n)/$1\tWebhookSecret string `json:"webhook_secret"`\n\tBackfillToken string `json:"backfill_token"`\n/' "$CONTROL_ROOT/modules/repositorysettings/types.go"
      perl -0pi -e 's/WebhookURL: base \+ "\/api\/v1\/webhooks\/" \+ r\.Provider/WebhookURL: base + "\/api\/v1\/webhooks\/" + r.Provider, WebhookSecret: r.WebhookSecretCiphertext, BackfillToken: r.BackfillTokenCiphertext/' "$CONTROL_ROOT/modules/repositorysettings/controller.go"
      expected='webhook_secret'
      ;;
  esac
  echo "CONTROL $control: identical seeded acceptance must fail ($expected)"
  if run_control "$CONTROL_ROOT" >"$CONTROL_ROOT/control.log" 2>&1; then cat "$CONTROL_ROOT/control.log"; echo "$control control unexpectedly passed" >&2; exit 1; fi
  cat "$CONTROL_ROOT/control.log"
  grep -F "$expected" "$CONTROL_ROOT/control.log" >/dev/null || { echo "$control control failed for an unintended reason" >&2; exit 1; }
  rm -rf "$CONTROL_ROOT"
  CONTROL_ROOT=
done
echo 'RESTORATION: production source restored; baseline rerun'
run_test
