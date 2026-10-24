# Status

## Now
- Measured 2026-10-24T01:18Z: main `8c366da408256e47531a73cd48dff1f64e4fef06`; latest main CI `40048643966` successful on preceding code head `0f26ec3`. PR #156 API CI successful on `85556534499dc11a1228f26b5101eec5f7f15db8`; returned for missing acceptance/evidence, send-back 1/2.
- Two detached workers verified: WS-b-r2 (OpenCode, API conformity #125–#129, same PR/branch) and W1e (OpenCode, ingestion smoke #149). Resume preserves original dirty API worktree and reconciles the separately published commit; scopes remain API versus scripts/docs.
- Roadmap planned through W17; freshly counted 71 root and 15 client open issues, none missing status labels. No owner decision. WS-d awaits WS-b and board #19/#113. Pass: `.agent-runs/coordinator/20261024-081634-pass.md`.

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
| W1e ingestion smoke | running | #149 | OpenCode W1e; committed fixture and cleanup proof |
| W2b board UI | merged | #115 | OpenCode; fixture browser/control evidence recorded |
| WS-a1 client bootstrap | merged | client #1 | OpenCode; complete |
| WS-a2 split | merged | #141 | OpenCode; complete |
| WS-c client conformity | merged | client #22 | OpenCode; env/browser checks recorded |
| WS runner dispatch | merged | #154 / #151 closed | OpenCode; one send-back; real detached client-root fixture accepted |
| WS-b API conformity | returned; retry running | #156 / #125–#129 | OpenCode WS-b-r2; send-back 1/2, complete acceptance and evidence |
| W2a / WS-d | blocked | #19/#113, #133/#134 | board API and WS-b precede real-data cross-repo smoke |
| W3–W17 | planned; dependency-blocked | GitHub issue board | dispatch in roadmap dependency order |

## Worker usage
Raw harness fields; USD unverified. Codex input includes cache; OpenCode input excludes cache. Historical measured totals and per-attempt returns remain in [the pre-compaction record](https://github.com/0Hoag/agent-ops/blob/808fdd3118eea980aa2763068db85811d5f2d99f/STATUS.md#usage-and-cost-by-wave); they are not fresh measurements.

| Attempt | Harness / model | Input | Cached input | Cache write | Output | Reasoning | Result |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| W1d-r5 | OpenCode / openai/gpt-6-luna low | 92,878 | 3,948,288 | 0 | 17,196 | 6,899 | accepted; #155 merged after one send-back |
| WS-b | Codex / gpt-reserve low | 4,831,960 | 4,654,336 | 0 | 14,132 | 1,881 | #156 returned; send-back 1/2 |
| WS-b-r2 | OpenCode / openai/gpt-6-luna low | pending | pending | pending | pending | pending | dispatched on same branch |
| W1e | OpenCode / openai/gpt-6-luna low | pending | pending | pending | pending | pending | dispatched; no send-back |

WS-b totals freshly read from the last 20 lines of `.agent-runs/WS-b.log`; W1d-r5 is historical accepted usage. Coordinator usage excluded. This pass inspected PR evidence and published API files, reran no acceptance environments, and returned missing worker evidence rather than supplying it.
