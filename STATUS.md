# Status

## Now
- Measured 2026-10-24T01:42Z: main `8ef65129b3ce905fb2ef49e84514d7ff3c874ef5`; exact-head main CI `40050782605` successful. PR #156 merged after final review; API content verified identical on main.
- Two detached workers verified: Codex W2a builds board/repository endpoints #19/#113; OpenCode WS-health fixes client #24 response-envelope mismatch in a client-root worktree.
- Roadmap planned through W17; freshly counted 66 root and 16 client open issues, none missing status labels. No owner decision. WS-d awaits W2a; #159 module audit follows W2a. Pass: `.agent-runs/coordinator/20261024-0142-pass.md`.

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
| WS-b API conformity | merged; module audit follow-up blocked | #156 / #125–#129 closed; #159 | OpenCode WS-b-r5; accepted after 2 send-backs; audit after W2a |
| W2a board API | running | #19/#113 | Codex W2a; board/repository endpoints and isolated SQL/HTTP acceptance |
| WS-health client | running | client #24 | OpenCode; unwrap actual API health envelope, fixture/control/browser proof |
| WS-d real-data smoke | blocked | #133/#134 | W2a precedes real-data cross-repo smoke |
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
| W2a | Codex / gpt-reserve low | pending | pending | pending | pending | pending | dispatched #19/#113 |
| WS-health | OpenCode / openai/gpt-6-luna low | pending | pending | pending | pending | pending | dispatched client #24 |
| W1e | OpenCode / openai/gpt-6-luna low | 47,992 | 1,782,656 | 0 | 13,009 | 0 | #157 returned; send-back 1/2 |
| W1e-r2 | OpenCode / openai/gpt-6-luna low | 29,272 | 537,344 | 0 | 4,539 | 929 | accepted; #157 merged after one send-back |

WS-b-r5 totals freshly read from its log tail; earlier completed totals are historical. Coordinator usage excluded. PR #156 tested head was 3cfe2dbb1fbc45ad3cfb0363363749465d028b07; coordinator make ci and diff check passed. Unrelated dirty worker STATUS.md preserved in Git stash before worktree removal.
