# Status

Baseline `[M]` on 2026-10-24: `git rev-parse HEAD` = `ec7a6941946f00c5a61d62a39139da19a682a68a`; no `api/`, `web/` or CI; `gh pr list` and `gh issue list --state all` returned `[]`. Both dispatched worktrees started from `70bef3aaf49a87080df0333c095cf92dfb47462c` (`git worktree list`). Marks: `[M]` measured with command and commit; `[?]` unverified; `[X]` withdrawn.

## Wave status

| Wave | Status | PR | Notes |
| --- | --- | --- | --- |
| W0a api | merged | [#4](https://github.com/0Hoag/agent-ops/pull/4) | #1 closed; coordinator repeated API gates on exact PR head, all passed |
| W0b web | merged | [#5](https://github.com/0Hoag/agent-ops/pull/5) | #2 closed; coordinator repeated full web gate on exact PR head, all passed |
| W0c ci | merged | [#27](https://github.com/0Hoag/agent-ops/pull/27) | #3 closed; OpenCode; independent review and failing control #39 verified |
| W1a scm | merged | [#101](https://github.com/0Hoag/agent-ops/pull/101) | #6–#11, #102–#107, #112 closed; Codex initial/remediation, OpenCode two focused follow-ups; 3 returns |
| W1b store | merged | [#100](https://github.com/0Hoag/agent-ops/pull/100) | #12–#16 and #108–#111 closed; Codex CLI; one return |
| W1c ingest | returned, W1cr3 running | [#114](https://github.com/0Hoag/agent-ops/pull/114) | W1cr2 stopped with only an uncommitted signature test; Codex retry preserves it and completes #118/#137 |
| W1d stale snapshots | queued | | #119; after W1c, before board API acceptance |
| W2b board UI | merged | [#115](https://github.com/0Hoag/agent-ops/pull/115) | #20/#21/#120 closed; merge confirmed by gh at 2026-10-23T20:49:54Z; W2br supplied fixture browser/mutation evidence |
| WS-a1 client bootstrap | returned, hosted control running | [client #1](https://github.com/0Hoag/agent-ops-client/pull/1) | #121–#123/#138; full coordinator gate passes; missing actual failing GitHub control run |

## Autopilot single pass — 2026-10-24

- `[M]` Latest pass inspected at 2026-10-23T21:18:04Z (2026-10-24 local): `git pull --ff-only` was already up to date at `a4cb6fde72c88d3a5710dc1fd2d8220c2133a557`, working tree clean; main CI `40025627009` completed successfully on that SHA. Open issues still cover the roadmap through W17; no owner decision is pending.
- `[M]` W1cr3 and WS-a1-control runners 54462/54467 and harnesses 54546/54549 are alive, elapsed about two minutes, with no final `.md` reports. Read only their last 20 log lines; neither is stale or reports a usage limit. W1cr3 logged a failed patch match in its existing signature test; its harness remains active and API PR #114 head remains `e35ab610b5525df8ce07949481295467eb3a634c`. No completed worker PR is ready for acceptance; no merge, restart or additional dispatch this pass (two-worker cap).
- `[M]` Client PR #1 advanced to `a50c5cccf481f6fdaec60742a1ed11dedf9687b4`, with successful Client CI `40025619826`. Coordinator independently inspected negative-control run `40025603133` at `4f947ad66aa93a4941edd4ea70de2bed161215a7`: frozen install succeeds, `pnpm typecheck` fails with TS2353/TS2339 for injected `HealthResponse.uptime`; control PR #3 is closed unmerged. Hosted rejection evidence now exists; final worker report, exact-current-head local gate and review remain for the next pass. Both active attempt token totals remain pending; no worker waiting inside this pass.

- `[M]` Synced main; a concurrent coordinator script update appeared during sync and was committed separately as `1abf9bf1cb8780ff4f1719b2a5c25c3805296265`. Working tree subsequently clean. Main CI run `40025300416` was queued at inspection; no green-main claim for that commit. Whole-roadmap planning remains in issues; no owner decision is pending.
- `[M]` API PR #114 still has head `e35ab610b5525df8ce07949481295467eb3a634c`. W1cr2 summary explicitly reports no rollback/retry or repeat-sync tests, no gates/smoke/control, and no commit/push. `git status --short` in its worktree shows modified `api/modules/webhook/service_test.go`; preserved for retry. Returned/commented without merging or claiming independently repeated API gates.
- `[M]` Client PR #1 head `4eb5314d86ac405c1015b20694b1457af034fc5b`, CI `40024311084` Client successful. Coordinator independently ran frozen install, typecheck, lint, tests and build in the clean client bootstrap clone at this exact head: exit 0, 3 tests, bundle-size warnings. Reviewed PR scope, workflow and runner diff; fully qualified closing references #121–#123 are present. No client app behavior changed by bootstrap; real API integration remains queued for WS-d.
- `[M]` Client negative control PR #2 is closed with zero check runs (`statusCheckRollup: []`); its head repository is the same `0Hoag/agent-ops-client`, contrary to the worker's fork explanation. Local TypeScript failure is not the required hosted CI rejection. Created #138 (`found-by:agent`), returned #1 and dispatched a hosted-control follow-up. No merge this pass. Root removal/transfers and downstream conformance remain behind prerequisites.
- `[M]` No worker processes at sync. Spawned detached W1cr3 with Codex (`gpt-reserve`, low) on the existing ingestion worktree, and WS-a1-control with OpenCode (`openai/gpt-6-luna`, low), implementing in the separate existing client clone. `pgrep` confirmed runners 54462/54467 and harness processes 54546/54549; briefs `briefs/W1cr3.md`, `briefs/WS-a1-control.md`. Both new attempts have usage pending; no wait inside this pass.

Roadmap through W17 is now split into deliverable issues #6–#99 (PR numbers occupy gaps), with scoped labels, measurable acceptance and dependencies. #98 covers the initial Claude Code transcript importer; #99 covers local Docker Compose deployment. Owner updates observed during this run: local Docker Compose deployment and Claude Code transcript ingestion are decided; W3 has no owner-decision blocker. Phase 2 W6–W17 is included in planning.

## Verification on PR heads

- `[M]` API head `a87b2ca85db7c6a70e57ea2ed21b521b4cfdfcb5`: coordinator ran `make ci` and `go vet ./... && golangci-lint run && go test ./... && go build ./...` in the detached `review-w0a/api` worktree, all exit 0; lint reports 0 issues. The database package now has 3 migration tests and handler tests pass. `DATABASE_URL='postgres://agent_ops:agent_ops_dev@127.0.0.1:1/agent_ops?sslmode=disable&connect_timeout=1' go run .` exits 1 for the intended connection failure. Independent review reproduced startup failure exiting 0 on the previous head; the worker fixed it in this head.
- `[M]` Worker API smoke on this head: dedicated Compose Postgres/API ports 55432/18080; health 200, readiness 200, DB stopped readiness 503, DB restarted readiness 200. Migration mutation (`value TEXT NOT NULL BROKEN`) makes database tests fail with SQL mismatch. Evidence: `.agent-runs/W0a-retry.jsonl`; these Docker/control results were recorded by the worker, not independently repeated by the coordinator.
- `[M]` Web head `ccba5da7e0a4c505411094a10e5dc35ba92d3782`: coordinator ran `pnpm install --frozen-lockfile && pnpm typecheck && pnpm lint && pnpm test && pnpm build` in detached `review-w0b/web`, all exit 0; 1 test passes. Build emits 2 bundle-size recommendations.
- `[M]` Web mutation control: detached `control-w0b` at the same head, change only the requested endpoint to `/wrong-control-endpoint`; `pnpm test` exits 1 and explicitly reports expected `/healthz`, actual `/wrong-control-endpoint`. The unmodified head passed before mutation; PR source was not changed.
- `[M]` Coordinator Playwright Chromium verification: `PLAYWRIGHT_BROWSERS_PATH=/Users/zeross/agent-ops-wt/feat/w0b-web/web/.agent-runs/browsers node .agent-runs/verify-web.cjs`. This PR's local dev server at `http://127.0.0.1:4189/` renders sidebar and `API status: ok`, with no page/console errors in the success case. A delayed HTTP fixture shows loading; a 503 fixture shows the error state. Screenshots: `.agent-runs/w0b-health-ok.png`, `.agent-runs/w0b-health-error.png`. Fixture responses match the shared API contract; this is not a browser-to-real-API integration claim. This closes the browser/control gaps left in the worker's report.
- `[M]` Coordinator repeated both full gates on the exact W0a/W0b heads during this session; both passed. PRs #4/#5 merged under owner authorization. File-scoped diffs of `origin/main` against their original heads (`api/ docker-compose.yml` and `web/`) were empty after merge.
- `[M]` W0c head `3ef3e235d9ee152cb26855b9c41e7cee1cb16bfc`: coordinator full API and web gates exit 0; independent read-only review found no blocker. CI run `40015869876` API/Web passed. Control #39 at measured head `39a71927668b2f6bed52dc749e15ec07563b3522`, run `40016097101`, Web fails at `HealthPage.tsx(4,9)` with TS2322 and exit 2; control closed unmerged. Worker mistyped the control SHA in its report; coordinator corrected PR metadata after measuring. PR #27 merged; file-scoped workflow diff against main is empty. Main commit `1994fbb` CI run `40016973013` completed successfully (`gh run watch --exit-status`).
- `[M]` W1b final head `90e5d231beb40153a06f588b326dc138bcfc9360`: coordinator reran full API gate, all exit 0. Entity-specific SQL tests now inspect conflict/update and lookup clauses; authoritative snapshots clear removed values. CI `40018262684` API/Web green. PR #100 merged, scoped diff of store/migrations/runner tests against main empty. Main CI `40018391525` green on merge `57a7a0b`. End-to-end Postgres ingestion is W1c acceptance, not claimed here.
- `[M]` W1a initial head `b32a2a3db4087958194f26b87d9e3132f4893ef0`: full API gate green, but coordinator temporary Go regressions failed for encoded repository slash, missing state=all, non-closing refs, merged_at state and actual status:in progress label. Independent review also found registry configuration impossible. Issues #102–#107 created and PR returned. Remediation head `bf5447bbfe363d93995c2b349b0d2207e3d9cbc7` passes coordinator full API gate and CI `40018308933`; read-only re-review resolved five findings but #105 still matches directly negated closing prose. Focused second remediation is running; PR not merged.
- `[M]` W1a final head `72ce5ab22a6a55435204d6789d453827985a3cb6`: coordinator full API gate passes; inspected occurrence-local negation handling and parser split (all changed Go files <=200 lines). CI `40019286910` API/Web green. PR #101 merged, scoped SCM diff against main empty. Main CI `40019487309` green on merge `1657c07`. Earlier intermediate-head note above is historical.
- `[M]` W1c head `3bb4b283ec30076f93d6cbcaf6c8e67af83f3a7e`: coordinator full API gate and CI `40019858775` pass, but no webhook or sync tests exist. Coordinator started isolated Docker Postgres on 55433 and actual PR API on 18082 with two fixture repositories/secrets. `.agent-runs/reproduce-w1c.py`: A payload signed with A ->400, same payload signed with B ->200 and persisted under A; same issues body with workflow_run header ->200 and pipeline external_id=0, empty SHA/status. SQL queries confirmed two raw rows and bad normalized records. PR returned for #116–#118. Independent review's stale snapshot finding is queued separately as #119.
- `[M]` W2b head `aae2a9c17c053ea9e3992bfd5e532aa0111be0ec`: coordinator full web gate exit 0 (3 tests), CI `40019931869` API/Web green. Worker omitted browser and mutation evidence; #120 tracks completion before merge.

## Usage and cost by wave

Source: totals printed by `scripts/run-worker*.sh` after completion (Codex: `turn.completed.usage`; OpenCode: `step_finish.part.tokens`).

| Wave | Harness | Worker | Model | Input | Cached input | Cache write | Output | Cache hit % | USD | Issue | Returns |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| W0a | Codex CLI 0.164.0 | worker | gpt-reserve | 3,355,793 | 3,245,184 | 0 | 32,033 | 96.70% | [?] unavailable | #1 | 1 |
| W0b | OpenCode 1.19.4 | worker | openai/gpt-6-luna | 101,706 | 1,440,128 | 0 | 13,887 | 93.40% | [?] unavailable | #2 | 2 |
| W0c | OpenCode 1.19.4 | worker | openai/gpt-6-luna | 19,347 | 330,496 | 0 | 3,760 | 94.47% | [?] unavailable | #3 | 0 |
| W1a, initial + first return | Codex CLI | worker | gpt-reserve | 3,283,440 | 3,149,184 | 0 | 31,325 | 95.91% | [?] unavailable | #6–#11, #102–#107 | 3 total across harnesses |
| W1a, second + third returns | OpenCode | worker | openai/gpt-6-luna | 59,233 | 546,048 | 0 | 5,744 | 90.21% | [?] unavailable | #105/#112 | included above |
| W1b | Codex CLI | worker | gpt-reserve | 2,241,029 | 2,133,760 | 0 | 24,148 | 95.21% | [?] unavailable | #12–#16, #108–#111 | 1 |
| W1c, initial only | Codex CLI | worker | gpt-reserve | 1,690,236 | 1,642,112 | 0 | 9,195 | 97.15% | [?] unavailable | #17/#18 | 2 total, latest running |
| W2b, initial only | OpenCode | worker | openai/gpt-6-luna | 36,243 | 168,064 | 0 | 4,969 | 82.26% | [?] unavailable | #20/#21 | 1 total, merged |
| W1cr, completed return | OpenCode | worker | openai/gpt-6-luna | 191,024 | 24,185,344 | 0 | 56,492 | 99.22% | [?] unavailable | #116–#118 | second return dispatched |
| W2br, completed verification | OpenCode | worker | openai/gpt-6-luna | 49,255 | 1,015,296 | 0 | 5,955 | 95.37% | [?] unavailable | #120 | 1 total |
| WS-a1, incomplete bootstrap | OpenCode | worker | openai/gpt-6-luna | 38,071 | 119,936 | 0 | 828 | 75.91% | [?] unavailable | #121–#123 | 1, retry running |
| W1cr2, incomplete signature follow-up | OpenCode | worker | openai/gpt-6-luna | 40,063 | 211,328 | 0 | 1,237 | 84.06% | [?] unavailable | #118/#137 | incomplete; Codex retry dispatched |
| WS-a1-retry, bootstrap PR | OpenCode | worker | openai/gpt-6-luna | 98,726 | 2,345,088 | 0 | 15,272 | 95.96% | [?] unavailable | #121–#123 | returned for #138 |

Latest completed-run totals were read from the last 20 lines of `.agent-runs/{W1cr2,WS-a1-retry}.log` this pass; reasoning was 0 for both. W1cr3 and WS-a1-control are separate attempts with totals pending. Earlier rows retain historical usage rather than including retries.

Measured after all five script invocations finished on 2026-10-24. Include every attempt, not only successful runs:

| Run | Input (raw) | Cached input | Cache write | Output | Reasoning |
| --- | ---: | ---: | ---: | ---: | ---: |
| W0a | 1,675,856 | 1,626,368 | 0 | 16,044 | 4,006 |
| W0a-retry | 1,679,937 | 1,618,816 | 0 | 15,989 | 3,773 |
| W0b | 24,573 | 363,648 | 0 | 4,756 | 320 |
| W0b-retry | 37,952 | 742,272 | 0 | 4,356 | 470 |
| W0b-review-fix | 39,181 | 334,208 | 0 | 4,775 | 408 |
| W0c | 19,347 | 330,496 | 0 | 3,760 | 497 |
| W1a | 2,217,847 | 2,121,216 | 0 | 21,894 | 3,792 |
| W1ar | 1,065,593 | 1,027,968 | 0 | 9,431 | 1,487 |
| W1b | 1,373,871 | 1,318,400 | 0 | 16,067 | 3,535 |
| W1br | 867,158 | 815,360 | 0 | 8,081 | 1,150 |
| W1ar2 | 34,663 | 255,488 | 0 | 2,685 | 191 |
| W1ar3 | 24,570 | 290,560 | 0 | 3,059 | 62 |
| W1c | 1,690,236 | 1,642,112 | 0 | 9,195 | 845 |
| W2b | 36,243 | 168,064 | 0 | 4,969 | 36 |

- Reproduce: sum `turn.completed.usage` in `.agent-runs/W0a{,-retry}.jsonl`; sum `step_finish.part.tokens` in `.agent-runs/W0b.jsonl`, `W0b-retry.jsonl`, `W0b-review-fix.jsonl` with `jq -s`. Script summaries are in the matching `.log` files.
- **Input is the raw harness field, not directly comparable across harnesses.** Codex `input_tokens` includes cached input: cache hit = cached/input. OpenCode input excludes cache reads/writes: cache hit = cache.read/(input + cache.read + cache.write). All 88 OpenCode events satisfy `tokens.total = input + cache.read + cache.write + output + reasoning` (verified with `jq -s`, zero mismatches). Normalized total prompt tokens: Codex **3,355,793**; OpenCode **1,541,834**. Codex input semantics follow [OpenAI usage documentation](https://developers.openai.com/api/docs/guides/prompt-caching#monitor-cache-performance).
- **Output is also the raw harness field.** Reasoning totals: Codex **7,779**, OpenCode **1,198**. Codex reports reasoning as a subset of output; OpenCode reports it separately, as confirmed by the total-token identity above. Normalized output including reasoning: Codex **32,033**; OpenCode **15,085**.
- OpenCode's 88 `step_finish` events report aggregate `part.cost = 0`; this is a harness-reported value, **not proof of zero billed cost**. Codex JSONL provides tokens but no billed USD. No verified tariff/billing mapping for these worker model aliases was supplied; actual USD is unknown rather than estimated from another model's price.
- Scope: worker script usage only. Coordinator and independent reviewer usage is unavailable here and excluded. Workloads differ, so these figures are not a controlled model benchmark.

## New agent failures (promote to FIVE-MINUTES.md if repeated)

- W0a: initial worker reported green gates despite startup failures returning exit 0; independent review and coordinator reproduction caught it. One return fixed startup handling, added migration tests/control and dev setup instructions.
- W0b: first attempt stopped with Rspack CSS parsing failure and no PR. First return fixed CSS/Tailwind and JSX runtime; second return completed module/router/layout composition and endpoint assertion. The coordinator supplied the remaining mutation and browser evidence after worker verification commands failed.
- W1: green gates missed acceptance behavior; promoted to FIVE-MINUTES.md. Codex could not write shared worktree Git metadata and used isolated temporary clones to commit/push. Original worker worktrees retain stale untracked snapshots; they are being preserved pending comparison rather than removed blindly.
