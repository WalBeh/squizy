# Changelog

All notable changes to squizy are documented here. Format loosely follows
[Keep a Changelog](https://keepachangelog.com/); versions use [SemVer](https://semver.org/).

## [Unreleased]

### Added
- Per-level latency detail block: full distributions (p50/p90/p99/max) for TTFT,
  TTFA, inter-token latency (ms), and end-to-end (send→last token). Surfaces the
  tail and the inter-token/end-to-end metrics the summary table omits — the
  numbers that matter most under saturation.
- `list` subcommand: enumerate models on an endpoint and flag non-chat targets
  (embeddings, OCR, rerankers).
- `run` subcommand: closed-loop virtual-user load test with concurrency sweep.
  - Synthetic prompts with controlled input/output token lengths.
  - `--unique-prompts` to defeat prefix caching (cache-realistic prefill).
  - Multi-turn conversations (`--turns`) with accumulating context.
  - Think-time between turns (`--think-time`, `--think-jitter`).
  - Per-level stop by `--duration` or `--requests`; warmup requests excluded.
  - Concurrency sweep (auto-doubling) with knee auto-stop on throughput
    plateau or error-rate threshold.
  - `--max-tokens` override for reasoning models so thinking doesn't starve
    the answer.
- Reasoning-aware metrics: thinking (`reasoning_content`) measured but
  separated from the answer; TTFT vs TTFA; generation tok/s (think+answer) as
  the primary throughput number.
- Aggregate vs per-user tok/s reported side by side; full distribution stats
  (mean, p50/p90/p99, min/max); live progress.
- Client-side timing that discards omlx `keepalive`/empty chunks; server
  `usage` token counts with a flagged local-estimate fallback.
- Optional auth via `--api-key` or `SQUIZY_API_KEY`; per-request timeout with
  non-fatal error counting.

### Notes
- Pure Go standard library, no external dependencies.
- Validated end-to-end against a live omlx endpoint.
