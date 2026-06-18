# Caddy Serves Frontend and Reverse Proxies API

The frontend container will use Caddy to serve built frontend static assets and reverse proxy API requests to the Go backend. This gives the browser a single entry point for the web UI and API while keeping backend and frontend services separate in Docker Compose.

## Consequences

Frontend routing, API base paths, and deployment documentation should assume Caddy is the public HTTP entry point. The Go backend does not need to serve frontend assets directly.
