# Single Public Frontend Entrypoint

The deployed system exposes only the frontend Caddy service to the host by default. Caddy serves the compiled UI and proxies `/api/*` requests to the backend, while document processing, OCR, LLM, and embedding services remain internal Compose-network services so users and documentation rely on one public HTTP surface.

