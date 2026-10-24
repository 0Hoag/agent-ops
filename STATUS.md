# Status

## Now
- Measured 2026-10-24T01:23Z: main `f5f031ca27d9a595778ca1a278d9081adc09f8e9`; latest main CI `40048643966` successful on code head `0f26ec3`. PRs #156/#157 exact-head API CI green; both await acceptance completion (send-back 1/2 each).
- Two detached OpenCode workers verified: WS-b-r3 resumes #125–#129 after r2 stopped; API branch equals published head, unrelated dirty STATUS preserved. W1e-r2 fixes reproduced SQL-count false pass #158 and completes #149; scopes remain API versus scripts/docs.
- Roadmap planned through W17; freshly counted 72 root and 15 client open issues, none missing status labels. No owner decision. WS-d awaits WS-b and board #19/#113. Pass: `.agent-runs/coordinator/20261024-082040-pass.md`.

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
| W1e ingestion smoke | returned; retry running | #157 / #149/#158 | OpenCode W1e-r2; send-back 1/2, exact SQL counts and control |
| W2b board UI | merged | #115 | OpenCode; fixture browser/control evidence recorded |
| WS-a1 client bootstrap | merged | client #1 | OpenCode; complete |
| WS-a2 split | merged | #141 | OpenCode; complete |
| WS-c client conformity | merged | client #22 | OpenCode; env/browser checks recorded |
| WS runner dispatch | merged | #154 / #151 closed | OpenCode; one send-back; real detached client-root fixture accepted |
| WS-b API conformity | returned; retry running | #156 / #125–#129 | OpenCode WS-b-r3; send-back 1/2, complete acceptance and evidence |
| W2a / WS-d | blocked | #19/#113, #133/#134 | board API and WS-b precede real-data cross-repo smoke |
| W3–W17 | planned; dependency-blocked | GitHub issue board | dispatch in roadmap dependency order |

## Worker usage
Raw harness fields; USD unverified. Codex input includes cache; OpenCode input excludes cache. Historical measured totals and per-attempt returns remain in [the pre-compaction record](https://github.com/0Hoag/agent-ops/blob/808fdd3118eea980aa2763068db85811d5f2d99f/STATUS.md#usage-and-cost-by-wave); they are not fresh measurements.

| Attempt | Harness / model | Input | Cached input | Cache write | Output | Reasoning | Result |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| W1d-r5 | OpenCode / openai/gpt-6-luna low | 92,878 | 3,948,288 | 0 | 17,196 | 6,899 | accepted; #155 merged after one send-back |
| WS-b | Codex / gpt-reserve low | 4,831,960 | 4,654,336 | 0 | 14,132 | 1,881 | #156 returned; send-back 1/2 |
| WS-b-r2 | OpenCode / openai/gpt-6-luna low | 34,976 | 326,400 | 0 | 1,281 | 0 | stopped after Git reconciliation; no API changes |
| WS-b-r3 | OpenCode / openai/gpt-6-luna low | pending | pending | pending | pending | pending | dispatched on same branch |
| W1e | OpenCode / openai/gpt-6-luna low | 47,992 | 1,782,656 | 0 | 13,009 | 0 | #157 returned; send-back 1/2 |
| W1e-r2 | OpenCode / openai/gpt-6-luna low | pending | pending | pending | pending | pending | dispatched; #158 exact-count acceptance |

WS-b-r2 and W1e totals freshly read from their log tails; earlier totals are historical. Coordinator usage excluded. This pass reproduced the smoke false pass with a synthetic two-row snapshot; no acceptance environment rebuilt.
