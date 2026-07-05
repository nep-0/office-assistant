# Runbook

## Start The Skeleton Stack

The project is designed for Compose-compatible Podman first, while staying compatible with Docker Compose where practical.

```sh
podman compose up --build
```

If using Docker Compose:

```sh
docker compose up --build
```

The frontend is available at `http://localhost:8080` by default. Set `FRONTEND_PORT` to use another host port.

## Smoke Check

With the stack running, verify the public frontend entrypoint and backend proxy:

```sh
bash scripts/smoke.sh
```

The smoke check requests the Caddy-served frontend, `/api/health`, and `/api/ready` through the single public entrypoint.

## Cloud Or Fake Provider Startup

The default Compose startup uses deterministic fake OpenAI-compatible providers so the full UI and backend can run without model credentials:

```sh
podman compose up --build
```

For cloud testing, keep the same containers but pass provider settings through environment variables before first boot:

```sh
OPENROUTER_API_KEY="..." bash scripts/setup-cloud.sh
```

If SQLite already contains provider settings, update them in the admin UI or reset the backend volume for a fresh first-boot seed.

`scripts/setup-cloud.sh` defaults to OpenRouter-compatible settings with `poolside/laguna-xs.2` for chat and `qwen/qwen3-embedding-8b` for embeddings. Override `CHAT_PROVIDER_BASE_URL`, `CHAT_MODEL`, `CHAT_API_KEY`, `EMBEDDING_PROVIDER_BASE_URL`, `EMBEDDING_MODEL`, or `EMBEDDING_API_KEY` for another OpenAI-compatible provider. Set `CONTAINER_ENGINE=docker` to use Docker Compose.

## Local Model Startup

The local model path uses the `local-models` Compose profile. Place GGUF files in a host model directory and point Compose at it:

```sh
ls /home/jeff/models/MiniCPM5-1B-Q4_K_M.gguf
ls /home/jeff/models/embeddinggemma-300m-qat-Q8_0.gguf
```

Start the stack with cloud providers disabled and both model endpoints set to the internal llama.cpp servers:

```sh
bash scripts/setup-local.sh
```

Docker Compose uses the same script with `CONTAINER_ENGINE=docker`:

```sh
CONTAINER_ENGINE=docker bash scripts/setup-local.sh
```

The default local profile uses `ghcr.io/ggml-org/llama.cpp:server-b9717`, `/home/jeff/models/MiniCPM5-1B-Q4_K_M.gguf` for chat, `/home/jeff/models/embeddinggemma-300m-qat-Q8_0.gguf` for embeddings, targets ordinary CPU deployment, sets `-ngl 0` for both llama.cpp servers, and uses conservative context defaults. Override `LLAMA_CPP_IMAGE`, `MODEL_DIR`, `LLM_GGUF`, `EMBEDDING_GGUF`, `LLM_CONTEXT_SIZE`, `LLM_GPU_LAYERS`, or `EMBEDDING_GPU_LAYERS` only when the deployment machine has the matching image, model files, memory, or GPU support.

Local models can take a while to load. `/api/ready` reports `degraded` with a provider message while the llama.cpp `/v1/models` endpoint is unreachable, then returns `ready` after both chat and embedding providers respond.

## Offline Smoke Check

After the local-model stack is ready, run the end-to-end offline smoke:

```sh
bash scripts/offline-smoke.sh
```

The script creates a small synthetic DOCX, creates or logs in as a local admin, creates a knowledge base, uploads the document, polls ingestion, asks a knowledge-base question, requires citations, and writes `artifacts/offline-smoke/report.json`. Keep cloud provider environment variables unset for this proof path except for the local URLs shown above.

## Model Provider Defaults

The backend seeds first-boot Dual-Mode Model Provider settings from environment variables:

- `CHAT_PROVIDER_BASE_URL`
- `CHAT_MODEL`
- `CHAT_API_KEY`
- `EMBEDDING_PROVIDER_BASE_URL`
- `EMBEDDING_MODEL`
- `EMBEDDING_API_KEY`

Admins can change the active chat and embedding provider settings in the UI after login. API keys stay backend-side, are masked in API responses, and are not written into frontend assets.

## Backend State

The backend owns persistent application state in its mounted volume. SQLite lives at `/data/office-assistant.db`, and uploaded original documents are stored under `/data/files` using internal storage identifiers rather than user filenames.
