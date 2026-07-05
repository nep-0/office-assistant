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
FAKE_PROVIDERS=false \
CHAT_PROVIDER_BASE_URL="https://openrouter.ai/api/v1" \
CHAT_MODEL="your-chat-model" \
CHAT_API_KEY="..." \
EMBEDDING_PROVIDER_BASE_URL="https://openrouter.ai/api/v1" \
EMBEDDING_MODEL="your-embedding-model" \
EMBEDDING_API_KEY="..." \
podman compose up --build
```

If SQLite already contains provider settings, update them in the admin UI or reset the backend volume for a fresh first-boot seed.

## Local Model Startup

The local model path uses the `local-models` Compose profile. Place GGUF files in a host model directory and point Compose at it:

```sh
mkdir -p models
# Copy your chat GGUF to models/chat.gguf.
# Copy your embedding GGUF to models/embedding.gguf.
```

Start the stack with cloud providers disabled and both model endpoints set to the internal llama.cpp servers:

```sh
MODEL_DIR="$PWD/models" \
LLM_GGUF="chat.gguf" \
EMBEDDING_GGUF="embedding.gguf" \
FAKE_PROVIDERS=false \
CHAT_PROVIDER_BASE_URL="http://llm:8083/v1" \
CHAT_MODEL="local-chat" \
EMBEDDING_PROVIDER_BASE_URL="http://embedding:8084/v1" \
EMBEDDING_MODEL="local-embedding" \
podman compose --profile local-models up --build
```

Docker Compose uses the same environment:

```sh
MODEL_DIR="$PWD/models" \
FAKE_PROVIDERS=false \
CHAT_PROVIDER_BASE_URL="http://llm:8083/v1" \
CHAT_MODEL="local-chat" \
EMBEDDING_PROVIDER_BASE_URL="http://embedding:8084/v1" \
EMBEDDING_MODEL="local-embedding" \
docker compose --profile local-models up --build
```

The default local profile targets ordinary CPU deployment. It sets `-ngl 0` for both llama.cpp servers and conservative context defaults. Override `LLM_CONTEXT_SIZE`, `LLM_GPU_LAYERS`, or `EMBEDDING_GPU_LAYERS` only when the deployment machine has enough memory or GPU support.

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
