# Changelog

All notable changes to squizy are documented here. Format loosely follows
[Keep a Changelog](https://keepachangelog.com/); versions use [SemVer](https://semver.org/).

## [Unreleased]

### Added
- Inline reasoning detection: models that emit thinking inside `content` ending
  in `</think>` (Nemotron/DeepSeek with no server-side reasoning parser) are now
  split correctly, with the `</think>` boundary detected *during* streaming so
  TTFA reflects the first real answer token (the thinking tax) rather than the
  first thinking token. Handles closing-tag-only output (template-opened think
  blocks) in addition to full `<think>…</think>`.
- `--system`: inject a system prompt (e.g. `detailed thinking on` to toggle
  Nemotron reasoning).
- `--task prose|reason`: workload prompt style. `reason` poses randomized
  step-by-step problems that trigger a reasoning model's thinking block, so
  reasoning-mode throughput/latency can be measured.
- Network baseline: `--net-probes` (default 5) measures transport latency
  (cold TCP connect + warm round-trip min/p50/p90) against `/health` (falling
  back to `/v1/models`) and prints a one-line floor at the top of the run, so
  the network tax under every reported latency is explicit. Uses httptrace, no
  curl dependency.
- SLO mode: `--ttft-slo` and `--e2e-slo` (durations, 0=off) report per-level
  attainment (% of requests meeting the objective, flagged when <100%) in the
  latency block, plus a verdict of the highest concurrency that still holds.
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

### Changed
- Readability pass on output: the per-level table and the latency detail block
  now use shared, right-aligned column layouts (percentiles as columns with a
  unit column) instead of slash-separated inline values.

### Notes
- Pure Go standard library, no external dependencies.
- Validated end-to-end against a live omlx endpoint.
