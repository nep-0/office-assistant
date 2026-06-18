# CPU-Only Deployment Targets

The project defines lightweight deployment as running without a GPU on commodity office hardware, not as running comfortably on the weakest laptop. The minimum target is 8 CPU cores and 16 GB RAM; the recommended target is 12-16 CPU cores and 32 GB RAM for smoother demos, batch import tests, OCR, embedding, and local LLM generation.

## Consequences

Performance reports should state which target machine was used. Optimization claims should be evaluated against CPU-only deployment, while acknowledging that OCR and local LLM generation may be slow on the minimum target.
