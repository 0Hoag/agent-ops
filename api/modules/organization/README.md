# Organizations and memberships

Routes use the verified JWT `sub` as caller identity. `GET /organizations` and detail/members reads require membership. Owners and admins manage member/viewer memberships; only owners manage admins. Owners cannot be granted through the API, and the last owner cannot be removed or demoted. Invalid tokens, nonmembers, unsupported roles, and forbidden role changes are rejected.
