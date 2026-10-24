# Client issue transfer mapping

The 18 client issues below were transferred from the root repository. During split follow-up, a prior worker
replaced several issue bodies with generic boilerplate; the deliverable-specific contracts were then restored
from the destination issues' `userContentEdits` histories. Transfer itself did not generate that boilerplate.
Paths were adapted for the standalone client (`pnpm` from its repository root and `docs/client-standards.md`).

Canonical client checkout: `/Users/zeross/agent-ops-client`.

| Root issue | Client issue | Wave | Restored acceptance | Restored dependency |
| --- | --- | --- | --- | --- |
| [#132](https://github.com/0Hoag/agent-ops/issues/132) | [#4](https://github.com/0Hoag/agent-ops-client/issues/4) | WS | Full web gate, HTTP-only service seam, preserved board behavior | WS-a/WS-b/WS-c/WS-d ordering and API dependency |
| [#131](https://github.com/0Hoag/agent-ops/issues/131) | [#5](https://github.com/0Hoag/agent-ops-client/issues/5) | WS | Typecheck/build plus env precedence fixture and `.env.example` | WS-a/WS-b/WS-c/WS-d ordering and API dependency |
| [#130](https://github.com/0Hoag/agent-ops/issues/130) | [#6](https://github.com/0Hoag/agent-ops-client/issues/6) | WS | Package scripts pass and CI pins pnpm/Node | WS-a/WS-b/WS-c/WS-d ordering and API dependency |
| [#89](https://github.com/0Hoag/agent-ops/issues/89) | [#7](https://github.com/0Hoag/agent-ops-client/issues/7) | W16 | Login/board/runs/alerts E2E, CI suite, mutation control | Earlier roadmap phase dependencies |
| [#82](https://github.com/0Hoag/agent-ops/issues/82) | [#8](https://github.com/0Hoag/agent-ops-client/issues/8) | W14 | Full gate plus fake-worker lifecycle browser evidence | Earlier roadmap phase dependencies |
| [#77](https://github.com/0Hoag/agent-ops/issues/77) | [#9](https://github.com/0Hoag/agent-ops-client/issues/9) | W13 | Create/trigger/inspect alert history in browser | Earlier roadmap phase dependencies |
| [#72](https://github.com/0Hoag/agent-ops/issues/72) | [#10](https://github.com/0Hoag/agent-ops-client/issues/10) | W12 | Metrics linked to fixture recomputation tests | Earlier roadmap phase dependencies |
| [#69](https://github.com/0Hoag/agent-ops/issues/69) | [#11](https://github.com/0Hoag/agent-ops-client/issues/11) | W11 | 10k-run seed and sub-second timeline measurement | Earlier roadmap phase dependencies |
| [#68](https://github.com/0Hoag/agent-ops/issues/68) | [#12](https://github.com/0Hoag/agent-ops-client/issues/12) | W11 | Query-parameter, empty/error/data browser states | Earlier roadmap phase dependencies |
| [#65](https://github.com/0Hoag/agent-ops/issues/65) | [#13](https://github.com/0Hoag/agent-ops-client/issues/13) | W10 | Versioned price editor, USD/VND browser and arithmetic tests | Earlier roadmap phase dependencies |
| [#59](https://github.com/0Hoag/agent-ops/issues/59) | [#14](https://github.com/0Hoag/agent-ops-client/issues/14) | W8 | GitHub/GitLab settings browser coverage and one-time secret display | Earlier roadmap phase dependencies |
| [#53](https://github.com/0Hoag/agent-ops/issues/53) | [#15](https://github.com/0Hoag/agent-ops-client/issues/15) | W7 | Viewer/admin browser behavior matches server permissions | Earlier roadmap phase dependencies |
| [#52](https://github.com/0Hoag/agent-ops/issues/52) | [#16](https://github.com/0Hoag/agent-ops-client/issues/16) | W7 | Tenant switch clears query data without stale records | Earlier roadmap phase dependencies |
| [#51](https://github.com/0Hoag/agent-ops/issues/51) | [#17](https://github.com/0Hoag/agent-ops-client/issues/17) | W7 | Login/logout and session-expiry browser coverage | Earlier roadmap phase dependencies |
| [#47](https://github.com/0Hoag/agent-ops/issues/47) | [#18](https://github.com/0Hoag/agent-ops-client/issues/18) | W6 | Generated OpenAPI TypeScript client replaces handwritten HTTP calls | Earlier roadmap phase dependencies |
| [#36](https://github.com/0Hoag/agent-ops/issues/36) | [#19](https://github.com/0Hoag/agent-ops-client/issues/19) | W5 | Loading/empty/error/data browser states and unknown metrics | W5a |
| [#30](https://github.com/0Hoag/agent-ops/issues/30) | [#20](https://github.com/0Hoag/agent-ops-client/issues/20) | W3 | Cache-hit and known/unknown dollar cost arithmetic by wave/worker/issue; loading/empty/error/data browser states | [W3a](https://github.com/0Hoag/agent-ops/issues/25); deployment and Claude usage-source decision resolved by [ROADMAP.md](../ROADMAP.md) |
| [#23](https://github.com/0Hoag/agent-ops/issues/23) | [#21](https://github.com/0Hoag/agent-ops-client/issues/21) | W2 | Signed webhook moves a card without reload; repository switch leaks no subscriptions | Board UI [#20](https://github.com/0Hoag/agent-ops/issues/20) and [#21](https://github.com/0Hoag/agent-ops/issues/21) are implemented by [PR #115](https://github.com/0Hoag/agent-ops/pull/115); [#19 board API](https://github.com/0Hoag/agent-ops/issues/19) and [#22 websocket API](https://github.com/0Hoag/agent-ops/issues/22) are separate prerequisites |

## Label audit

Every destination issue `#4`-`#21` has exactly one status, priority, type, and wave label, plus `area:client`.
Status values were audited against prerequisites; resolved owner decisions are not blockers. Board API work remains
distinct from the completed W2b/W2c client UI deliverables.
