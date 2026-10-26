# Transcript parsers

This package parses the local JSONL records produced by the Codex, OpenCode, and Claude workers. Each parser returns deterministic, ID-sorted session metadata and token counters; it never retains prompt, tool, or message content. Missing optional usage fields are zero.

`ParseCodex` consumes `turn.completed`. Codex `input_tokens` includes cached input, while `cached_input_tokens` is reported separately for observability. Consumers must not add cached input to input a second time. Identity is `session_id`, then `thread_id`, then the active `thread.started.thread_id` for the published runner shape where later turns omit identity.

`ParseOpenCode` consumes `step_finish` (and the equivalent `step-finish` spelling), using `sessionID` and `part.tokens`. OpenCode input and `cache.read`/`cache.write` are separate source counters and may be normalized by the caller. Text and tool events are ignored.

`ParseClaude` consumes assistant records with `sessionId` and deduplicates repeated streamed representations by message `uuid`. Cache creation/read counters map to `CacheWrite`/`CachedInput`. Timestamps are only used when valid RFC3339 values are present; malformed timestamps do not prevent usage parsing.

Malformed JSON, records that cannot identify a usage-bearing session, scanner/read failures, and nil readers return errors. The package does not read files itself, so callers can provide an opened local transcript and handle path policy.
