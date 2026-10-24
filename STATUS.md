# Status

## Now
- Measured 2026-10-24T01:07Z: main `1bf7b6ab0428c6c789a85b8c44b59df0dbcac39c`; merge CI `40048232321` successful. Client main CI `40047158042` successful at `2d8667eaec5ce35b387fc6f3711c214e8de82554`.
- #154 merged with pinned head, scoped main diff empty, #151 closed and clean worker worktree removed. W1d-r5 remains active on #155; current head `a01672de236a4f6cb7d078517927cec3b4de47f1` has successful CI `40048072350`, final report/evidence pending.
- Roadmap planned through W17; 72 root and 15 client open issues have status labels. No owner decision. Serialize remaining API work behind #153; WS-d depends on WS-b and board #19/#113. Pass details: `.agent-runs/coordinator/20261024-080437-pass.md`.

## Waves
| Wave | State | PR / issues | Harness; next action |
| --- | --- | --- | --- |
| W0a API | merged | #4 | Codex; complete |
| W0b client foundation | merged | #5 | OpenCode; complete |
| W0c CI | merged | #27 | OpenCode; hosted negative control verified |
| W1a SCM | merged | #101 | Codex + OpenCode; complete |
| W1b store | merged | #100 | Codex; complete |
| W1c ingestion | merged | #114 | Codex + OpenCode; signed HTTP/PostgreSQL acceptance recorded |
| W1d stale snapshots | merged; acceptance follow-up running | #150, #155 / #153 | OpenCode W1d-r5; #155 returned once; wait for worker report |
| W1e ingestion smoke | blocked | #149 | queued after #153; committed fixture and cleanup proof |
| W2b board UI | merged | #115 | OpenCode; fixture browser/control evidence recorded |
| WS-a1 client bootstrap | merged | client #1 | OpenCode; complete |
| WS-a2 split | merged | #141 | OpenCode; complete |
| WS-c client conformity | merged | client #22 | OpenCode; env/browser checks recorded |
| WS runner dispatch | merged | #154 / #151 closed | OpenCode; one send-back; real detached client-root fixture accepted |
| WS-b API conformity | queued; API serialization | #125–#129 | after active #153; read company sources, preserve migration versions |
| W2a / WS-d | blocked | #19/#113, #133/#134 | board API and WS-b precede real-data cross-repo smoke |
| W3–W17 | planned; dependency-blocked | GitHub issue board | dispatch in roadmap dependency order |

## Worker usage
Raw harness fields; USD unverified. Codex input includes cache; OpenCode input excludes cache. Historical measured totals and per-attempt returns remain in [the pre-compaction record](https://github.com/0Hoag/agent-ops/blob/808fdd3118eea980aa2763068db85811d5f2d99f/STATUS.md#usage-and-cost-by-wave); they are not fresh measurements.

| Attempt | Harness / model | Input | Cached input | Cache write | Output | Reasoning | Result |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| W1d-r5 | OpenCode / openai/gpt-6-luna low | pending | pending | pending | pending | pending | active; #155 returned once |
| WS-runner-r2 | OpenCode / openai/gpt-6-luna low | 20,837 | 357,248 | 0 | 2,906 | 0 | accepted; #154 merged |
| client dispatch fixture | OpenCode / worker configuration | 5,389 | 4,736 | 0 | 212 | 0 | accepted fixture; separate session |

Latest completed totals read from the last 20 lines of `.agent-runs/WS-runner-r2.log`; nested fixture totals from the worker worktree's `.agent-runs/fixture.tokens.json`. Coordinator usage excluded. No new API/client product gate or real-data smoke claim this pass.
