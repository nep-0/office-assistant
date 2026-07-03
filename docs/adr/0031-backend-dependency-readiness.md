# Backend Dependency Readiness

The backend exposes overall health and per-dependency readiness for document processing, OCR, chat model, and embedding model services. The UI surfaces degraded provider status with actionable messages, and optional dependencies such as OCR are allowed to remain unavailable until a workflow needs them.

