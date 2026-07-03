# Static Frontend Runtime

The runtime frontend container is a Caddy service that serves compiled frontend assets and reverse-proxies backend API requests. Frontend build tooling may be used during development, but it is kept out of the final runtime Compose deployment to keep the deployed system simple and locally explainable.

