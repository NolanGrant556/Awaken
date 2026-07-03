# Context Compression Smoke

Use this smoke after changes to compaction, transcript restore, or any prompt-window behavior that should preserve older facts across a compact boundary.

## Command

```bash
go test ./src/server -run TestCompactCanaryFactsSurviveThreadRestoreSmoke -count=1 -v
```

## What it checks

- Real provider call through the server runtime.
- Small context budget that forces compact before the first provider request.
- Canary token survives the compact summary.
- A fresh server instance can restore the same thread from transcript and answer the canary recall query.

## Expected result

- Pass when `LLM_API_KEY` is configured and the provider returns the canary token on the second run.
- Skip when `LLM_API_KEY` is missing.
