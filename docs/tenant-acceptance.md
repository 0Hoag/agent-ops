# W7M tenant acceptance

Run `scripts/tenant-acceptance.sh` from the repository root. It creates a uniquely named disposable Compose project, runs `TestW7MTenantAcceptance` against its real PostgreSQL instance, and removes the project and volume on every exit. Set `W7M_TENANT_POSTGRES_PORT` when the default port is occupied.

The fixture has two organizations, accounts and memberships, one repository per tenant, issues, pull requests, pipeline runs and agent runs. The test uses the production `registerRoutes` wiring and signed organization tokens.

| Route / seam | Tenant A assertion | Cross-tenant assertion |
| --- | --- | --- |
| `GET /repositories` | total is 1 and own repository is present | other tenant is absent from list and total |
| `GET /board` | own issue/PR/run is present | direct foreign repository is 404; foreign records are absent |
| `GET /insights` | own repository is readable | foreign repository is 404 |
| `GET /agent-runs` | own records are listable | foreign repository and issue filters return empty |
| `GET /agent-runs/aggregate` | own aggregate is scoped | foreign repository aggregate is empty |
| `GET /board/ws` admission | own repository upgrades | foreign repository does not upgrade |
| WebSocket delivery | own hub event arrives | an event for the other repository does not arrive |
| membership | selected token works while membership exists | deleting membership returns 401 |

The configured machine-write binding is exercised by the production controller configuration (`tenant/a`); any request for an unconfigured repository must be rejected before persistence. This opt-in acceptance does not claim browser coverage; that is owned by the separate client repository smoke.

The script also runs two independent source-copy controls against the same seeded database and inputs. The board control removes the organization predicates from the disposable `RepositoryExists` and `ListIssues` paths and requires the foreign board request to fail with both an HTTP 404 mismatch and the leaked `tenant-b issue` diagnostic. The machine control removes the configured-repository guard from the disposable controller and requires the foreign write to fail with an HTTP 401 mismatch. The production files are never edited; their SHA-256 hashes are captured before the controls and checked before the restored baseline run.

`scripts/smoke.sh` defaults `CLIENT_DIR` to `.agent-runs/w7r-smoke-client` inside this worktree. It owns the Compose project, PostgreSQL volume, and preview process tree, and verifies that its containers, volumes, and client listener are absent from both success and failure cleanup paths.
