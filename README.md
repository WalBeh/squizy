# squizy

Load-test OpenAI-compatible chat endpoints (Ollama, vLLM, MLX / `mlx_lm.server`, omlx, …) and
report **honest token throughput** under simulated concurrent users.

A single tok/s number is a lie: a server can sustain thousands of tokens/sec aggregate while each
user feels a fraction of that. squizy always reports both, plus the latency users actually
perceive, and it sweeps concurrency to find where the server saturates.

## Build

```sh
go build -o squizy .
```

Pure standard library — no dependencies. Cross-platform (`GOOS=… GOOS=windows …`).

## Usage

List models on an endpoint (takes the guesswork out of model names; flags non-chat models):

```sh
squizy list --base-url http://localhost:8000/v1 --api-key KEY
```

Run a throughput sweep:

```sh
squizy run --base-url http://localhost:8000/v1 --api-key KEY \
  --model Qwen3.6-27B-MLX-4bit \
  --input-tokens 512 --output-tokens 256 \
  --duration 30s --max-users 64
```

API key can also come from `SQUIZY_API_KEY`. Local endpoints usually need none.

### What it measures

- **Generation tok/s** (think + answer) — the true rate the hardware produces tokens. Primary.
- **Aggregate vs per-user** — system capacity vs what one user experiences, side by side at each
  concurrency level.
- **TTFT / TTFA** — time to first token, and (for reasoning models) time to first *answer* token,
  with the thinking tax shown as `think%`.
- **Full distribution** — mean, p50/p90/p99, min/max.

### How load is generated

Closed-loop virtual users: each sends a request, waits for the full streamed response, pauses for
think-time, then repeats. Multi-turn conversations accumulate context (`--turns`). Concurrency
doubles (1, 2, 4, …) until aggregate throughput plateaus or errors climb — the saturation knee.

### Key flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--input-tokens` | 512 | target prompt input length |
| `--output-tokens` | 256 | target answer length (steers the prompt) |
| `--max-tokens` | auto | hard generation cap — **raise for reasoning models** so thinking doesn't starve the answer |
| `--turns` | 1 | turns per conversation (multi-turn when >1) |
| `--think-time` | 1s | pause between a user's turns (`--think-jitter` randomizes) |
| `--unique-prompts` | true | unique prefix per request to defeat prefix caching |
| `--start-users` / `--max-users` | 1 / 64 | sweep bounds |
| `--no-sweep` | false | test only `--start-users` |
| `--duration` / `--requests` | 30s / – | per-level stop condition |
| `--warmup` | 1 | discarded warmup requests (excludes cold model-load) |
| `--timeout` | 120s | per-request timeout |
| `--knee-gain` / `--knee-error-rate` | 0.10 / 0.05 | sweep auto-stop thresholds |

See [DESIGN.md](DESIGN.md) for metric definitions and the reasoning behind each decision.
