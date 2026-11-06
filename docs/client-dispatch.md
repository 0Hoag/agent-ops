# Detached client dispatch

From the agent-ops coordinator checkout, start or resume an OpenCode worker in an existing client clone/worktree:

```bash
scripts/run-worker-opencode.sh --target-repo /Users/zeross/agent-ops-client \
  --worktree /Users/zeross/agent-ops-client WS-c client-worker /absolute/path/to/self-contained-brief.md
```

The brief must be self-contained and readable from the coordinator checkout; the dispatcher embeds its contents in the prompt before launching OpenCode. Keep the brief outside the client worktree so dispatch does not add coordinator instructions to the client repository.

Use an authorized client checkout (canonical path `/Users/zeross/agent-ops-client`; implementation worktrees may be below `/Users/zeross/agent-ops-wt`). The explicit target-repo/worktree form resolves harness helpers from agent-ops, runs OpenCode from the existing client worktree, and writes `.agent-runs` under the client target repository. It does not create, reset, clean, or copy configuration into the client checkout. Existing dirty edits are retained. A fresh invocation resumes from the worktree's current state; use a new wave name if run artifacts should not be overwritten.

Coordinator artifacts: `.agent-runs/<wave>.log` (detached output), `.jsonl` (events), `.md` (final text), and `.tokens.json` (full token totals). Check the logged requested `WORKTREE`/`REMOTE`, final report, and token object. Wrong-root validation: pass a repository whose origin is not `agent-ops-client`; the runner exits 2 before starting OpenCode.
