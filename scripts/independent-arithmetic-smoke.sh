#!/usr/bin/env bash
set -Eeuo pipefail

# The existing smoke harness owns unique Compose resources, authentication, and
# cleanup. Arithmetic mode stops immediately after the focused API assertions.
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
export SMOKE_ARITHMETIC_ONLY=1
export SMOKE_API_PORT=${SMOKE_API_PORT:-18082}
export POSTGRES_HOST_PORT=${POSTGRES_HOST_PORT:-15433}
export SMOKE_CLIENT_PORT=${SMOKE_CLIENT_PORT:-4174}
export SMOKE_ARTIFACT_DIR=${SMOKE_ARTIFACT_DIR:-"$ROOT/.agent-runs/independent-arithmetic"}
export JWT_SECRET=${JWT_SECRET:-independent-arithmetic-smoke-jwt-secret}
export REPOSITORY_ENCRYPTION_KEY=${REPOSITORY_ENCRYPTION_KEY:-01234567890123456789012345678901}
exec "$ROOT/scripts/smoke.sh"
