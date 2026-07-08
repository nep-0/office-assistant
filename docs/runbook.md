# Runbook

## Start The Local Stack

The project is designed for Compose-compatible Podman first, while staying compatible with Docker Compose where practical. The default Compose file starts local llama.cpp chat and embedding services.

```sh
podman compose up --build
```

If using Docker Compose:

```sh
docker compose up --build
```

The frontend is available at `http://localhost:8080` by default. Edit `compose.yaml` if another host port, image, model directory, or model filename is needed.

## Smoke Check

With the stack running, verify the public frontend entrypoint and backend proxy:

```sh
bash scripts/smoke.sh
```

The smoke check requests the Caddy-served frontend, `/api/health`, and `/api/ready` through the single public entrypoint.

## Start From GHCR Images

After GitHub Actions publishes images, run the local-model stack without local builds:

```sh
podman compose -f compose.images.yaml up
```

The image Compose file uses `:latest` images by default. Edit `compose.images.yaml` to use another image tag, branch tag, release tag, or `sha-*` tag.

Use the same remote override with published images when testing against a cloud provider:

```sh
podman compose -f compose.images.yaml -f compose.remote.yaml up
```

## Cloud Provider Startup

The remote override uses OpenRouter-compatible provider settings with `poolside/laguna-xs.2` for chat and `qwen/qwen3-embedding-8b` for embeddings. Before first boot, edit the backend environment in `compose.remote.yaml` and set the chat and embedding API keys:

```sh
podman compose -f compose.yaml -f compose.remote.yaml up --build
```

If SQLite already contains provider settings, update them in the admin UI or reset the backend volume for a fresh first-boot seed.

Set `CHAT_PROVIDER_BASE_URL`, `CHAT_MODEL`, `CHAT_API_KEY`, `EMBEDDING_PROVIDER_BASE_URL`, `EMBEDDING_MODEL`, and `EMBEDDING_API_KEY` directly in `compose.remote.yaml` for another OpenAI-compatible provider.

## Local Model Startup

The default Compose file expects these GGUF files:

```sh
ls /home/jeff/models/MiniCPM5-1B-Q4_K_M.gguf
ls /home/jeff/models/embeddinggemma-300m-qat-Q8_0.gguf
```

Start the stack:

```sh
podman compose up --build
```

Docker Compose uses the same file:

```sh
docker compose up --build
```

The default local services use `ghcr.io/ggml-org/llama.cpp:server-b9717`, `/home/jeff/models/MiniCPM5-1B-Q4_K_M.gguf` for chat, and `/home/jeff/models/embeddinggemma-300m-qat-Q8_0.gguf` for embeddings. Edit `compose.yaml` if the deployment machine uses a different image, model directory, or model filename.

Local models can take a while to load. `/api/ready` reports `degraded` with a provider message while the llama.cpp `/v1/models` endpoint is unreachable, then returns `ready` after both chat and embedding providers respond.

## Offline Smoke Check

After the local stack is ready, run the end-to-end offline smoke:

```sh
bash scripts/offline-smoke.sh
```

The script creates a small synthetic DOCX, creates or logs in as a local admin, creates a knowledge base, uploads the document, polls ingestion, asks a knowledge-base question, requires citations, and writes `artifacts/offline-smoke/report.json`.

## OCR Service

The OCR service uses PaddleOCR's PP-OCR pipeline on CPU by default:

```sh
podman compose up
```

PaddleOCR, PaddleX, and ModelScope files are cached in named volumes mounted at `/root/.cache`, `/root/.paddlex`, and `/root/.modelscope`. The OCR container disables SELinux process labeling because rootless Podman named volumes can otherwise receive a different MCS label than the recreated OCR container. The first OCR request may take longer while models are prepared.

## Model Provider Defaults

The backend seeds first-boot Dual-Mode Model Provider settings from its container environment declared in `compose.yaml` or the active override file:

- `CHAT_PROVIDER_BASE_URL`
- `CHAT_MODEL`
- `CHAT_API_KEY`
- `CHAT_REQUEST_TIMEOUT`
- `EMBEDDING_PROVIDER_BASE_URL`
- `EMBEDDING_MODEL`
- `EMBEDDING_API_KEY`

`CHAT_REQUEST_TIMEOUT` defaults to `10m` in Compose. Increase it in the Compose file for low-end local hardware or intentionally long generations.

Admins can change the active chat and embedding provider settings in the UI after login. API keys stay backend-side, are masked in API responses, and are not written into frontend assets.

## Backend State

The backend owns persistent application state in its mounted volume. SQLite lives at `/data/office-assistant.db`, and uploaded original documents are stored under `/data/files` using internal storage identifiers rather than user filenames.
