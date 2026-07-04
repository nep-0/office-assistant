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
