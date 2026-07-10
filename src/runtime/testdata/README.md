# v1 transcript accident fixture

`v1_tool_messages_without_tool_events.jsonl` is a desensitized minimal v1 transcript.

Facts locked:

1. Message records contain assistant `tool_calls` and tool result messages for `apply_patch` (exact duplicate payloads) and `shell` (`exit_code=1`, `is_error=false`).
2. Event records only include text + turn/run terminal types.
3. No `TOOL_CALL_*`, `TOOL_RESULT`, or `ARTIFACT_*` events are present.

Phase 2 `LegacyTranscriptAdapter` must recover tool cards from messages without inventing missing artifacts.
