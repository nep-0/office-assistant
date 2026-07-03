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

