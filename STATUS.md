# Status

## Now
- Measured 2026-10-24T01:13Z: main `0f26ec3636458370d672eff00f5b6efb882da608`; merge CI `40048643966` successful. #155 merged pinned to `d898f43a055dc3e813b43605d4247a35e8a2b348`, scoped main diff empty; #153 closed and worktree removed.
- Two detached workers verified: WS-b (Codex, API conformity #125–#129) and W1e (OpenCode, ingestion smoke #149). Their API versus scripts/docs write scopes are separate; both start from the measured main.
- Roadmap planned through W17; 71 root and 15 client open issues have status labels. No owner decision. WS-d awaits WS-b and board #19/#113. Pass details: `.agent-runs/coordinator/20261024-081008-pass.md`.

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
| WS-b API conformity | running | #125–#129 | Codex WS-b; company-source parity, preserve migration versions |
| W2a / WS-d | blocked | #19/#113, #133/#134 | board API and WS-b precede real-data cross-repo smoke |
| W3–W17 | planned; dependency-blocked | GitHub issue board | dispatch in roadmap dependency order |

## Worker usage
Raw harness fields; USD unverified. Codex input includes cache; OpenCode input excludes cache. Historical measured totals and per-attempt returns remain in [the pre-compaction record](https://github.com/0Hoag/agent-ops/blob/808fdd3118eea980aa2763068db85811d5f2d99f/STATUS.md#usage-and-cost-by-wave); they are not fresh measurements.

| Attempt | Harness / model | Input | Cached input | Cache write | Output | Reasoning | Result |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| W1d-r5 | OpenCode / openai/gpt-6-luna low | 92,878 | 3,948,288 | 0 | 17,196 | 6,899 | accepted; #155 merged after one send-back |
| WS-b | Codex / gpt-reserve low | pending | pending | pending | pending | pending | dispatched; no send-back |
| W1e | OpenCode / openai/gpt-6-luna low | pending | pending | pending | pending | pending | dispatched; no send-back |

Completed totals read from the last 20 lines of `.agent-runs/W1d-r5.log`. Coordinator usage excluded. Coordinator reran focused sync/webhook tests and diff hygiene only; full gates, PostgreSQL acceptance and controls are worker evidence in #155.
