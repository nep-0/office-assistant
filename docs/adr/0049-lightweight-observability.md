# Lightweight Observability

The backend emits structured logs for requests, jobs, provider calls, and errors using correlation IDs, and persists lightweight workflow metrics needed for evaluation. Metrics include ingestion duration, chunk counts, embedding duration, retrieval latency, generation latency, and token counts when available, without requiring a full Prometheus or Grafana stack.

