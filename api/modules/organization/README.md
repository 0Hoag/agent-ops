# Organizations and memberships

Routes use the verified JWT `sub` as caller identity. `GET /organizations` and detail/members reads require membership. Owners and admins manage member/viewer memberships; only owners manage admins. Owners cannot be granted through the API, and the last owner cannot be removed or demoted. Invalid tokens, nonmembers, unsupported roles, and forbidden role changes are rejected.

All routes are prefixed with `/api/v1/organizations`:

| Method | Suffix | Success | Rejections |
| --- | --- | --- | --- |
| POST | (none) | 201 organization (creator atomically becomes owner) | 400 invalid/blank name; 401 invalid/missing JWT; 500 persistence failure |
| GET | (none) | 200 caller's organizations with caller memberships | 401; 500 persistence failure |
| GET | `/{organization_id}` | 200 organization and caller membership | 401; 404 nonmember; 500 persistence failure |
| GET | `/{organization_id}/members` | 200 memberships | 401; 404 nonmember; 500 persistence failure |
| POST | `/{organization_id}/members` | 204 | 400 invalid body; 401; 403 role denied; 404 nonmember; 409 membership conflict |
| PATCH | `/{organization_id}/members/{account_id}` | 204 | 400 invalid body; 401; 403 role denied; 404 nonmember; 409 last owner/membership conflict |
| DELETE | `/{organization_id}/members/{account_id}` | 204 | 401; 403 role denied; 404 nonmember; 409 last owner/membership conflict |

Add takes `account_id` and `role`; patch takes `role`. Owner/admin may manage member/viewer;
only owner may change or delete an existing admin, including demotion to member/viewer.
Only owner may demote/delete another owner, provided an owner remains. No caller may assign
owner, even an owner. Unsupported roles return 403 (missing required role returns 400).
Mutation requests serialize on the organization row, including authorization and owner counting.
Unknown target memberships and database constraint violations currently return generic 409.

## Real PostgreSQL acceptance

Set `ORGANIZATION_TEST_DATABASE_URL` to an **owned disposable PostgreSQL database** whose user
can create schemas, then run `go test -count=1 -v ./modules/organization` from `api/`.
Tests apply the real embedded migrations inside unique schemas and drop only those schemas.
Without this variable the integration tests explicitly skip; normal unit gates are not evidence
of PostgreSQL acceptance. The tests exercise real Gin requests, JWT verification, SQL writes,
all 60 actor/target/action combinations, cross-organization denials, rollback, FKs, role and
uniqueness constraints, owner-assignment denial and last-owner protection.
