# API development

## Prerequisites

- Go 1.26 or newer
- Docker with the Compose plugin
- `golangci-lint` for `make ci`

## Local development

From this directory, copy the example environment file and start Postgres:

```sh
cp .env.example .env
docker compose -f ../docker-compose.yml up -d
make dev
```

The API listens on `http://localhost:8080`. Check it with:

```sh
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

The Compose environment is used by the Postgres container; it is not automatically loaded into a host process. The `.env` file is loaded by the API, so `make dev` must be run from `api/`.

If port 5432 is busy, choose another host port and use the same port in `DATABASE_URL`:

```sh
POSTGRES_PORT=55432 docker compose -f ../docker-compose.yml up -d
DATABASE_URL='postgres://agent_ops:agent_ops_dev@localhost:55432/agent_ops?sslmode=disable' HTTP_ADDR=:18080 make dev
```

If port 8080 is busy, set `HTTP_ADDR` to another host port. Stop only this stack with `docker compose -f ../docker-compose.yml down`; do not stop unrelated containers.

## Checks

```sh
make ci
go vet ./...
go test ./...
go build ./...
```
