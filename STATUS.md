# Status

## Now
- Measured 2026-10-24T01:33Z: main `4ee8cbe64fba1ee63a8ded40b157ab0d9c445307` (status-only); latest executed main CI `40049816825` successful on code head `3533b17c2394d5ae860114cf352db1daed1669ad`.
- One detached OpenCode worker verified: WS-b-r5 continues #125–#129 on PR #156 after Codex r4 could not commit through its sandbox; API edits/tests preserved. Final send-back remains 2/2; published-head CI does not cover those pending edits.
- Roadmap planned through W17; freshly counted 70 root and 15 client open issues, none missing status labels. No owner decision. WS-d awaits WS-b and board #19/#113; other API work would overlap. Pass: `.agent-runs/coordinator/20261024-0133-pass.md`.

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
| WS-b API conformity | returned; publication retry running | #156 / #125–#129 | OpenCode WS-b-r5; send-back 2/2, publish preserved API work and evidence |
| W2a / WS-d | blocked | #19/#113, #133/#134 | board API and WS-b precede real-data cross-repo smoke |
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
| WS-b-r5 | OpenCode / openai/gpt-6-luna low | pending | pending | pending | pending | pending | same-branch publication/acceptance continuation |
| W1e | OpenCode / openai/gpt-6-luna low | 47,992 | 1,782,656 | 0 | 13,009 | 0 | #157 returned; send-back 1/2 |
| W1e-r2 | OpenCode / openai/gpt-6-luna low | 29,272 | 537,344 | 0 | 4,539 | 929 | accepted; #157 merged after one send-back |

WS-b-r4 totals freshly read from its log tail; earlier totals are historical. Coordinator usage excluded. Published PR remains at 85556534499dc11a1228f26b5101eec5f7f15db8; no merge this pass. Worker must attach evidence for its new tested head.
