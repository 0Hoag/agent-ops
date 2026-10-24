# Status

## Now
- Measured 2026-10-24T05:10Z: main `6c402e5`; code-main CI `40064906909` successful after #163 merged, closing #162. Confirmed smoke script identical to accepted PR head. No merge this pass.
- Restarted stopped WS-d as detached OpenCode WS-d-r2 (#133/#134), preserving pushed implementation; unblocked #159 and resumed WS-audit-r2 on existing PR #161 (returned 1/2) for current-main gates, successful signed smoke and raw output.
- Fresh board audit: 64 root and 15 client open issues, none missing status labels, none todo; two detached worker runners verified. No owner decision. Pass: `.agent-runs/coordinator/20261024-0510-pass.md`.

## Waves
| Wave | State | PR / issues | Harness; next action |
| --- | --- | --- | --- |
| W0a API | merged | #4 | Codex; complete |
| W0b client foundation | merged | #5 | OpenCode; complete |
| W0c CI | merged | #27 | OpenCode; hosted negative control verified |
| W1a SCM | merged | #101 | Codex + OpenCode; complete |
| W1b store | merged | #100 | Codex; complete |
| W1c ingestion | merged | #114 | Codex + OpenCode; signed HTTP/PostgreSQL acceptance recorded |
| W1d stale snapshots | merged; acceptance complete | #150, #155 / #153 closed | OpenCode W1d-r5; one send-back resolved |
| W1e ingestion smoke | merged; acceptance complete | #157 / #149/#158 closed | OpenCode W1e-r2; exact SQL counts/control and cleanup accepted |
| W2b board UI | merged | #115 | OpenCode; fixture browser/control evidence recorded |
| WS-a1 client bootstrap | merged | client #1 | OpenCode; complete |
| WS-a2 split | merged | #141 | OpenCode; complete |
| WS-c client conformity | merged | client #22 | OpenCode; env/browser checks recorded |
| WS runner dispatch | merged | #154 / #151 closed | OpenCode; one send-back; real detached client-root fixture accepted |
| WS-b API conformity | merged; module audit resumed | #156 / #125–#129 closed; #161/#159 | OpenCode WS-audit-r2; send-back 1/2, current-main evidence pending |
| W2a board API | merged; acceptance complete | #160 / #19/#113 closed | OpenCode W2a-r3; SQL/HTTP/isolation/control evidence accepted |
| WS-health client | merged | client #25 / #24 closed | OpenCode WS-health-r2; accepted after one send-back |
| WS-d real-data smoke | resumed; implementation pushed, evidence pending | #133/#134 | OpenCode WS-d-r2; clean-worktree/browser/control evidence and PR due |
| WS signed-smoke repair | merged | #163 / #162 closed | OpenCode WS-smoke-envelope; exact-head main CI green, content verified |
| W3–W17 | planned; dependency-blocked | GitHub issue board | dispatch in roadmap dependency order |

## Worker usage
Raw harness fields; USD unverified. Codex input includes cache; OpenCode input excludes cache. Historical measured totals and per-attempt returns remain in [the pre-compaction record](https://github.com/0Hoag/agent-ops/blob/808fdd3118eea980aa2763068db85811d5f2d99f/STATUS.md#usage-and-cost-by-wave); they are not fresh measurements.

| Attempt | Harness / model | Input | Cached input | Cache write | Output | Reasoning | Result |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| W1d-r5 | OpenCode / openai/gpt-6-luna low | 92,878 | 3,948,288 | 0 | 17,196 | 6,899 | accepted; #155 merged after one send-back |
| WS-b | Codex / gpt-reserve low | 4,831,960 | 4,654,336 | 0 | 14,132 | 1,881 | #156 returned; send-back 1/2 |
| WS-b-r2 | OpenCode / openai/gpt-6-luna low | 34,976 | 326,400 | 0 | 1,281 | 0 | stopped after Git reconciliation; no API changes |
| WS-b-r3 | OpenCode / openai/gpt-6-luna low | 8,230 | 7,552 | 0 | 393 | 0 | stopped; gates only, no implementation |
| WS-b-r4 | Codex / gpt-reserve low | 2,819,530 | 2,731,904 | 0 | 11,426 | 2,222 | stopped at sandbox Git lock; API work preserved |
| WS-b-r5 | OpenCode / openai/gpt-6-luna low | 76,651 | 2,891,776 | 0 | 11,165 | 0 | #156 accepted after 2 send-backs; merged |
| W2a | Codex / gpt-reserve low | 1,306,432 | 1,244,928 | 0 | 9,890 | 1,493 | stopped at sandbox Git denial; work preserved |
| WS-health | OpenCode / openai/gpt-6-luna low | 11,579 | 82,432 | 0 | 1,743 | 0 | client #25 returned; send-back 1/2 |
| W2a-r2 | OpenCode / openai/gpt-6-luna low | 34,970 | 1,079,040 | 0 | 4,672 | 0 | #160 returned; send-back 1/2 |
| WS-health-r2 | OpenCode / openai/gpt-6-luna low | 33,049 | 191,872 | 0 | 3,053 | 0 | client #25 accepted and merged |
| W2a-r3 | OpenCode / openai/gpt-6-luna low | 80,833 | 1,261,952 | 0 | 9,317 | 908 | accepted; #160 merged after one send-back |
| W1e | OpenCode / openai/gpt-6-luna low | 47,992 | 1,782,656 | 0 | 13,009 | 0 | #157 returned; send-back 1/2 |
| W1e-r2 | OpenCode / openai/gpt-6-luna low | 29,272 | 537,344 | 0 | 4,539 | 929 | accepted; #157 merged after one send-back |
| WS-audit | OpenCode / openai/gpt-6-luna low | 66,692 | 1,193,600 | 0 | 5,505 | 0 | #161 returned 1/2; blocked on #162 |
| WS-smoke-envelope | OpenCode / openai/gpt-6-luna low | 38,289 | 177,920 | 0 | 1,787 | 0 | #163 merged; #162 closed |

WS-smoke-envelope totals freshly read from its log tail; earlier completed totals are historical. Coordinator usage excluded. WS-d's stopped attempt has no completed log summary; usage unknown, not zero. Resumed attempts have no completed totals yet. #161 remains at `cc0ef2c124bbb815a7d7360972c20ac07b2ddbfe` with green CI on the old head; refreshed signed-smoke and gate evidence is due before review.
