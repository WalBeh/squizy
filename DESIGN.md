# squizy — design

A cross-platform Go CLI that load-tests OpenAI-compatible chat endpoints (Ollama, vLLM,
MLX / `mlx_lm.server`, omlx, …) and reports **honest token-throughput** under simulated
concurrent users.

The guiding principle: a single number is a lie. A server can sustain 2000 tok/s aggregate
while each user feels 30 tok/s. squizy always reports both, plus the latency that users
actually perceive, and it sweeps concurrency to find where the server saturates.

---

## 1. Scope

- **In:** OpenAI-compatible `/v1/chat/completions` (streaming) and `/v1/models`.
- **In:** closed-loop virtual users, multi-turn conversations, synthetic controlled prompts,
  concurrency sweep, reasoning-aware metrics, full distribution stats, text output.
- **Out (for now):** non-chat endpoints (embeddings/OCR), JSON/CSV export, web UI, multi-target
  comparison in one run, non-OpenAI protocols.

---

## 2. Core decisions (locked)

| Area | Decision |
|------|----------|
| Headline metric | Aggregate system tok/s **and** per-user perceived tok/s, side by side, at every concurrency level. |
| Latency | TTFT + inter-token latency, full distribution (mean, p50/p90/p99, min/max). |
| Load model | Closed-loop virtual users: send → await full stream → think-time → repeat. |
| Conversation | Multi-turn; context accumulates, so prefill cost climbs each turn. |
| Workload | Synthetic prompts, controlled input + output token lengths. `--unique-prompts` toggles cache realism. |
| Concurrency | Auto-doubling sweep (1,2,4,8,…) until the knee; bounded by `--max-users`. |
| Stop (per level) | `--duration` or `--requests`. |
| Warmup | N warmup requests fired and discarded before measuring. |
| Reasoning | Counted but **separated**: thinking (`reasoning_content`) vs answer (`content`); first-thinking-token vs first-answer-token tracked independently. |
| Token source | Server `usage` via `stream_options.include_usage`; local estimate fallback, flagged. |
| Timing | **Client-side wall-clock only.** Server `created` timestamps ignored (coarse, 1s granularity). |
| Auth | Optional `--api-key` / `SQUIZY_API_KEY`. |
| Errors | Per-request timeout; failures counted and reported, never fatal. |
| Scope | One endpoint + model per invocation. |

### Server quirks already observed (omlx, port 8000)

- First SSE chunk is `{"model":"keepalive", delta:{content:""}}` — a placeholder sent
  immediately during prefill, **before any real token**. Must be discarded or TTFT reads ~0.
- A role-only delta (`delta:{role:"assistant"}`) precedes content — also not a token.
- `reasoning_content` is a first-class delta field here. Other servers may instead inline
  `<think>…</think>` in `content`; we support that as a fallback.
- **TTFT clock starts at request send; first *real* token = first non-empty `content` or
  `reasoning_content` delta from a non-keepalive chunk.**

---

## 3. Metric definitions (precise)

Per request we capture client-side timestamps:

- `t_send` — request written to the wire.
- `t_first_think` — first non-empty `reasoning_content` delta (nil if none).
- `t_first_answer` — first non-empty `content` delta.
- `t_last` — final token chunk.
- `n_think`, `n_answer` — token counts (server `usage` if present, else local estimate).

Derived:

- **TTFT** = `t_first_token − t_send`, where `t_first_token = min(t_first_think, t_first_answer)`.
  This is what a user waits before *anything* appears.
- **TTFA** (time-to-first-answer) = `t_first_answer − t_send`. For reasoning models this is the
  real "useful" wait; the gap `TTFA − TTFT` is thinking time.
- **Generation tok/s (per request, PRIMARY)** = `(n_think+n_answer) / (t_last − t_first_token)`.
  The true rate the hardware emits tokens. For non-reasoning models this equals the answer rate;
  for reasoning models it's the honest "what the GPU is doing" number. *Chosen as primary after
  live testing showed answer-only collapses to ~0 on heavy-thinking models when `max_tokens`
  truncates the answer — see note below.*
- **Answer (decode) tok/s (per request, secondary)** = `n_answer / (t_last − t_first_answer)`.
  The "useful" output rate a user perceives once thinking ends. Unmeasurable when the answer is
  truncated, so reported separately and short answers are excluded.
- **Inter-token latency** = mean gap between consecutive content tokens (per request), summarized
  across requests.
- **Per-user perceived tok/s** = decode tok/s as seen by one virtual user (answer tokens).
- **Aggregate tok/s** (the level-wide number) = `Σ tokens emitted by all users / wall-clock window`,
  measured over the steady-state window of that level. Computed for answer-only and
  incl.-thinking.

Output shortfall: if `n_answer < 0.7 × target_output`, the request's decode sample is flagged
"short" (noisier) and counted separately in the report footnote.

---

## 4. Concurrency & timing model

```
level = 1, 2, 4, 8, ... (doubling), capped by --max-users
for each level C:
    spawn C virtual users
    each user loops (until level stop condition):
        build next turn (synthetic prompt; turn 1 = input_len, later turns append)
        send streaming request, record sample
        think-time pause (fixed or jittered)
    barrier: all users stop at level's duration/requests boundary
    aggregate samples for the level
    decide knee: stop if aggregate gain < 10% vs prev level OR error rate > 5%
```

- **Warmup** runs once before the sweep (or before each level — default: once, configurable),
  discarded.
- **Steady-state window:** to keep ramp-up/ramp-down out of the aggregate, the level's aggregate
  tok/s is computed over the window between the first request *completing* and the stop signal,
  not the whole wall-clock including spin-up. (Per-request samples are unaffected.)
- **Closed-loop** means offered load self-limits: a slow server simply produces fewer
  requests/sec, never a backlog. That's the realism we want.

---

## 5. CLI surface

```
squizy list   --base-url URL [--api-key KEY]
              → table of model ids; flags likely non-chat (embed/ocr/rerank) by id heuristic.

squizy run    --base-url URL --model NAME [flags]
```

Key `run` flags (with defaults):

| Flag | Default | Meaning |
|------|---------|---------|
| `--base-url` | (required) | server root incl. `/v1`, e.g. `http://localhost:8000/v1` |
| `--model` | (required) | model id (from `list`) |
| `--api-key` / `SQUIZY_API_KEY` | "" | bearer token |
| `--input-tokens` | 512 | target prompt input length (turn 1) |
| `--output-tokens` | 256 | target answer length; sets `max_tokens` ceiling + steers prompt |
| `--turns` | 1 | turns per conversation (multi-turn when >1) |
| `--think-time` | 1s | pause between a user's turns |
| `--think-jitter` | 0.5 | ± fraction randomizing think-time |
| `--unique-prompts` | true | unique prefix per request (defeat prefix cache) |
| `--max-users` | 64 | sweep ceiling |
| `--start-users` | 1 | sweep start |
| `--duration` | 30s | per-level run time (mutually exclusive with --requests) |
| `--requests` | 0 | per-level request count (overrides duration when >0) |
| `--warmup` | 1 | discarded warmup requests |
| `--timeout` | 120s | per-request timeout |
| `--knee-gain` | 0.10 | min aggregate improvement to keep ramping |
| `--knee-error-rate` | 0.05 | error rate that stops the sweep |
| `--no-sweep` | false | test only `--start-users` (single level) |

---

## 6. Package layout

```
squizy/
  main.go                 // wire cobra/flag, dispatch list/run
  internal/
    config/   config.go   // RunConfig struct, flag parsing, validation
    client/
      client.go           // HTTP, /v1/models
      stream.go           // SSE decode, keepalive/empty filtering, delta split
      types.go            // chunk/usage structs
    workload/
      prompt.go           // synthetic prompt gen (unique/identical, target lengths)
      tokens.go           // local token estimate (tiktoken-ish heuristic or lib)
    engine/
      vuser.go            // one virtual user's loop
      level.go            // run one concurrency level
      sweep.go            // doubling + knee detection
    metrics/
      sample.go           // per-request RequestSample
      stats.go            // percentiles, aggregation
      level.go            // LevelResult
    report/
      text.go             // formatted summary
      progress.go         // live progress
```

Concurrency primitives: `context.Context` for cancellation/timeout, `sync.WaitGroup` for the VU
barrier, a buffered channel of `RequestSample` from VUs to the level collector, `errgroup` not
required. No global state; everything flows through `RunConfig` and result structs.

---

## 7. Token counting

- Preferred: server `usage` (`prompt_tokens`, `completion_tokens`; some servers also break out
  reasoning). Request it via `stream_options.include_usage:true` — arrives in a final chunk.
- Fallback: local estimate. We avoid a heavy tokenizer dependency initially and use a
  calibrated chars/token heuristic (~3.7 chars/token, model-family-agnostic) to count emitted
  text, **flagging the run "estimated"**. A real tokenizer can slot in behind the same interface
  later. We always know `n_think`/`n_answer` separately because we count the two delta streams
  independently regardless of source.

---

## 8. Output sketch

```
squizy run — Qwen3.6-27B-MLX-4bit @ http://localhost:8000/v1
config: input=512 output=256 turns=1 think=1s±50% unique-prompts=true
        per-level=30s warmup=1 timeout=120s  token-source=server

users   req   err   agg tok/s   per-user tok/s   TTFT p50/p90    TTFA p50/p90    decode p50
   1     28    0       41.3          41.3        0.21s / 0.28s   1.9s / 2.4s       42.1
   2     54    0       79.8          40.1        0.22s / 0.31s   2.0s / 2.6s       41.0
   4    101    0      151.2          38.0        0.25s / 0.40s   2.3s / 3.1s       39.2
   8    176    0      243.6          30.9        0.39s / 0.71s   3.4s / 5.0s       31.5
  16    210    2      271.0          17.1        0.82s / 1.9s    6.1s / 9.8s       18.0   ← knee
sweep stopped: aggregate gain 11% but error rate 0.9% … (or knee reason)

knee: ~8 users — aggregate plateaus at ~244 tok/s, per-user halves from 41 → 17.
reasoning: thinking 71% of tokens; mean think time 3.2s before first answer token.
notes: 4 requests short of output target (<70%), excluded from decode p50.
```

---

## 9. Build / test plan

1. `go build ./...`, `go vet`.
2. `squizy list` against `localhost:8000` — expect the 6 known models.
3. `squizy run --no-sweep --start-users 1 --requests 3` smoke test (gemma, dense, fast).
4. Small sweep on Qwen3.6-27B to exercise reasoning split + keepalive handling.
5. Verify TTFT is *not* ~0 (keepalive correctly skipped) and reasoning tokens are separated.

## 10. Notes from live testing (omlx, port 8000)

- **Every model on this server emits `reasoning_content`** — omlx applies a thinking template
  globally (even "gemma" runs 100% thinking). Confirmed the keepalive skip (TTFT ≈ 0.9s, not 0)
  and reasoning separation work.
- **`max_tokens` must accommodate thinking + answer.** With a tight cap, thinking eats the whole
  budget and the answer truncates to ~0, making answer-only metrics blank. Hence the
  `--max-tokens` override (default auto = `output×1.5+64`; raise it for reasoning models).
- **Primary metric is generation tok/s** (think+answer), not answer-only — see §3.
- **Aggregate window** = span from first request sent to last token received across successful
  requests (not a sub-window); a steady-state sub-window degenerated to a near-zero divisor at low
  request counts. At 1 user, aggregate ≈ per-user, as it must.
- Observed saturation shape on gemma-4-12B-8bit: aggregate 8→17→22 tok/s as users go 1→2→4,
  while per-user holds ~8–9 then drops to ~6 and TTFT climbs — the capacity/experience divergence.
