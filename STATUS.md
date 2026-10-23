# Status

Baseline `[M]` on 2026-10-24: `git rev-parse HEAD` = `ec7a6941946f00c5a61d62a39139da19a682a68a`; no `api/`, `web/` or CI; `gh pr list` and `gh issue list --state all` returned `[]`. Both dispatched worktrees started from `70bef3aaf49a87080df0333c095cf92dfb47462c` (`git worktree list`). Marks: `[M]` measured with command and commit; `[?]` unverified; `[X]` withdrawn.

## Wave status

| Wave | Status | PR | Notes |
| --- | --- | --- | --- |
| W0a api | merged | [#4](https://github.com/0Hoag/agent-ops/pull/4) | #1 closed; coordinator repeated API gates on exact PR head, all passed |
| W0b web | merged | [#5](https://github.com/0Hoag/agent-ops/pull/5) | #2 closed; coordinator repeated full web gate on exact PR head, all passed |
| W0c ci | merged | [#27](https://github.com/0Hoag/agent-ops/pull/27) | #3 closed; OpenCode; independent review and failing control #39 verified |
| W1a scm | in progress | | #6–#11; Codex CLI / gpt-reserve; `feat/w1a-scm` |
| W1b store | in progress | | #12–#16; Codex CLI / gpt-reserve; `feat/w1b-store` |

Roadmap through W17 is now split into deliverable issues #6–#99 (PR numbers occupy gaps), with scoped labels, measurable acceptance and dependencies. #98 covers the initial Claude Code transcript importer; #99 covers local Docker Compose deployment. Owner updates observed during this run: local Docker Compose deployment and Claude Code transcript ingestion are decided; W3 has no owner-decision blocker. Phase 2 W6–W17 is included in planning.

## Verification on PR heads

- `[M]` API head `a87b2ca85db7c6a70e57ea2ed21b521b4cfdfcb5`: coordinator ran `make ci` and `go vet ./... && golangci-lint run && go test ./... && go build ./...` in the detached `review-w0a/api` worktree, all exit 0; lint reports 0 issues. The database package now has 3 migration tests and handler tests pass. `DATABASE_URL='postgres://agent_ops:agent_ops_dev@127.0.0.1:1/agent_ops?sslmode=disable&connect_timeout=1' go run .` exits 1 for the intended connection failure. Independent review reproduced startup failure exiting 0 on the previous head; the worker fixed it in this head.
- `[M]` Worker API smoke on this head: dedicated Compose Postgres/API ports 55432/18080; health 200, readiness 200, DB stopped readiness 503, DB restarted readiness 200. Migration mutation (`value TEXT NOT NULL BROKEN`) makes database tests fail with SQL mismatch. Evidence: `.agent-runs/W0a-retry.jsonl`; these Docker/control results were recorded by the worker, not independently repeated by the coordinator.
- `[M]` Web head `ccba5da7e0a4c505411094a10e5dc35ba92d3782`: coordinator ran `pnpm install --frozen-lockfile && pnpm typecheck && pnpm lint && pnpm test && pnpm build` in detached `review-w0b/web`, all exit 0; 1 test passes. Build emits 2 bundle-size recommendations.
- `[M]` Web mutation control: detached `control-w0b` at the same head, change only the requested endpoint to `/wrong-control-endpoint`; `pnpm test` exits 1 and explicitly reports expected `/healthz`, actual `/wrong-control-endpoint`. The unmodified head passed before mutation; PR source was not changed.
- `[M]` Coordinator Playwright Chromium verification: `PLAYWRIGHT_BROWSERS_PATH=/Users/zeross/agent-ops-wt/feat/w0b-web/web/.agent-runs/browsers node .agent-runs/verify-web.cjs`. This PR's local dev server at `http://127.0.0.1:4189/` renders sidebar and `API status: ok`, with no page/console errors in the success case. A delayed HTTP fixture shows loading; a 503 fixture shows the error state. Screenshots: `.agent-runs/w0b-health-ok.png`, `.agent-runs/w0b-health-error.png`. Fixture responses match the shared API contract; this is not a browser-to-real-API integration claim. This closes the browser/control gaps left in the worker's report.
- `[M]` Coordinator repeated both full gates on the exact W0a/W0b heads during this session; both passed. PRs #4/#5 merged under owner authorization. File-scoped diffs of `origin/main` against their original heads (`api/ docker-compose.yml` and `web/`) were empty after merge.
- `[M]` W0c head `3ef3e235d9ee152cb26855b9c41e7cee1cb16bfc`: coordinator full API and web gates exit 0; independent read-only review found no blocker. CI run `40015869876` API/Web passed. Control #39 at measured head `39a71927668b2f6bed52dc749e15ec07563b3522`, run `40016097101`, Web fails at `HealthPage.tsx(4,9)` with TS2322 and exit 2; control closed unmerged. Worker mistyped the control SHA in its report; coordinator corrected PR metadata after measuring. PR #27 merged; file-scoped workflow diff against main is empty. Main commit `1994fbb` CI run `40016973013` completed successfully (`gh run watch --exit-status`).

## Usage and cost by wave

Source: totals printed by `scripts/run-worker*.sh` after completion (Codex: `turn.completed.usage`; OpenCode: `step_finish.part.tokens`).

| Wave | Harness | Worker | Model | Input | Cached input | Cache write | Output | Cache hit % | USD | Issue | Returns |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| W0a | Codex CLI 0.164.0 | worker | gpt-reserve | 3,355,793 | 3,245,184 | 0 | 32,033 | 96.70% | [?] unavailable | #1 | 1 |
| W0b | OpenCode 1.19.4 | worker | openai/gpt-6-luna | 101,706 | 1,440,128 | 0 | 13,887 | 93.40% | [?] unavailable | #2 | 2 |
| W0c | OpenCode 1.19.4 | worker | openai/gpt-6-luna | 19,347 | 330,496 | 0 | 3,760 | 94.47% | [?] unavailable | #3 | 0 |

Measured after all five script invocations finished on 2026-10-24. Include every attempt, not only successful runs:

| Run | Input (raw) | Cached input | Cache write | Output | Reasoning |
| --- | ---: | ---: | ---: | ---: | ---: |
| W0a | 1,675,856 | 1,626,368 | 0 | 16,044 | 4,006 |
| W0a-retry | 1,679,937 | 1,618,816 | 0 | 15,989 | 3,773 |
| W0b | 24,573 | 363,648 | 0 | 4,756 | 320 |
| W0b-retry | 37,952 | 742,272 | 0 | 4,356 | 470 |
| W0b-review-fix | 39,181 | 334,208 | 0 | 4,775 | 408 |
| W0c | 19,347 | 330,496 | 0 | 3,760 | 497 |

- Reproduce: sum `turn.completed.usage` in `.agent-runs/W0a{,-retry}.jsonl`; sum `step_finish.part.tokens` in `.agent-runs/W0b.jsonl`, `W0b-retry.jsonl`, `W0b-review-fix.jsonl` with `jq -s`. Script summaries are in the matching `.log` files.
- **Input is the raw harness field, not directly comparable across harnesses.** Codex `input_tokens` includes cached input: cache hit = cached/input. OpenCode input excludes cache reads/writes: cache hit = cache.read/(input + cache.read + cache.write). All 88 OpenCode events satisfy `tokens.total = input + cache.read + cache.write + output + reasoning` (verified with `jq -s`, zero mismatches). Normalized total prompt tokens: Codex **3,355,793**; OpenCode **1,541,834**. Codex input semantics follow [OpenAI usage documentation](https://developers.openai.com/api/docs/guides/prompt-caching#monitor-cache-performance).
- **Output is also the raw harness field.** Reasoning totals: Codex **7,779**, OpenCode **1,198**. Codex reports reasoning as a subset of output; OpenCode reports it separately, as confirmed by the total-token identity above. Normalized output including reasoning: Codex **32,033**; OpenCode **15,085**.
- OpenCode's 88 `step_finish` events report aggregate `part.cost = 0`; this is a harness-reported value, **not proof of zero billed cost**. Codex JSONL provides tokens but no billed USD. No verified tariff/billing mapping for these worker model aliases was supplied; actual USD is unknown rather than estimated from another model's price.
- Scope: worker script usage only. Coordinator and independent reviewer usage is unavailable here and excluded. Workloads differ, so these figures are not a controlled model benchmark.

## New agent failures (promote to FIVE-MINUTES.md if repeated)

- W0a: initial worker reported green gates despite startup failures returning exit 0; independent review and coordinator reproduction caught it. One return fixed startup handling, added migration tests/control and dev setup instructions.
- W0b: first attempt stopped with Rspack CSS parsing failure and no PR. First return fixed CSS/Tailwind and JSX runtime; second return completed module/router/layout composition and endpoint assertion. The coordinator supplied the remaining mutation and browser evidence after worker verification commands failed.
