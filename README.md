# squizy

Load-test OpenAI-compatible chat endpoints (Ollama, vLLM, MLX, omlx) and report **honest token
throughput** under simulated concurrent users — aggregate vs per-user tok/s, latency, and the
saturation knee.

## Build

```sh
go build -o squizy .
```

Pure standard library, no dependencies.

## Usage

```sh
# list models on an endpoint
squizy list --base-url http://localhost:8000/v1 --api-key KEY

# run a throughput sweep
squizy run --base-url http://localhost:8000/v1 --api-key KEY \
  --model Qwen3.6-27B-MLX-4bit --input-tokens 512 --output-tokens 256 \
  --duration 30s --max-users 64
```

API key may also come from `SQUIZY_API_KEY`. Run `squizy run -h` for all flags.

See [DESIGN.md](DESIGN.md) for metric definitions and rationale, and [CHANGES.md](CHANGES.md) for
the changelog.
