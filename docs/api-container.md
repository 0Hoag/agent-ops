# Production API container

The API image is a two-stage, CGO-disabled build. The final Alpine image contains the static API binary, CA certificates, and `wget` for its `/readyz` healthcheck. It runs as numeric UID/GID `10001` and includes embedded SQL migrations.

Build from the repository root:

```bash
docker build -t agent-ops-api:local ./api
```

Run it against PostgreSQL (required environment names):

```bash
docker run --rm -p 8080:8080 \
  -e DATABASE_URL='postgres://USER:PASSWORD@HOST:5432/DB?sslmode=disable' \
  -e JWT_SECRET='a-disposable-jwt-secret' \
  -e REPOSITORY_ENCRYPTION_KEY='01234567890123456789012345678901' \
  -e HTTP_ADDR=':8080' agent-ops-api:local
```

Readiness calls `/readyz` and requires a successful database ping. Run the isolated Postgres/API fixture with:

```bash
bash scripts/test-api-container.sh
```
