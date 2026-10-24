# Board API

The board module exposes repository selectors and paginated issue boards. It maps store models to public DTOs so webhook secret environment references never leave the API. Pull requests are linked by closing issue number and repository; pipeline runs are linked by head SHA and repository.

## WebSocket invalidation

Clients connect to `GET /api/v1/board/ws?repository_id=<repository UUID>`. The server sends JSON `{"type":"board.invalidate","repository_id":"<repository UUID>"}` after a newly accepted webhook transaction commits. The selector is the same UUID used by `GET /api/v1/board?repository_id=...`.
Subscriptions are repository-scoped; rejected, rolled-back, failed-commit, and duplicate webhook deliveries emit no event. Slow clients are disconnected and do not block webhook ingestion.
