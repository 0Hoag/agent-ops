# Operational alerts

`Evaluator` deterministically evaluates four rules from an explicit timestamp and repository-scoped snapshot:
low cache hit, silent active worker, failed completed CI on `main`, and stale open unmerged PR. Cache hit is
`cached_input_tokens / (input_tokens + cached_input_tokens)`; zero denominator never fires. Thresholds are strict:
exactly at a threshold does not fire, while a value beyond it does.

The in-memory evaluator deduplicates by organization, repository, rule, and subject. A qualifying snapshot repeats
only once; a non-qualifying snapshot clears that incident so a later recovery and new failure can alert again.
Callers such as #76 should pass their persisted observation snapshot and replace this state with an atomic history
store before delivery. This package does not schedule work, persist alerts, or send notifications.
