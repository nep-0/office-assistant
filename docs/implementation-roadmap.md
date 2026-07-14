# Implementation Roadmap

This roadmap turns the settled architecture decisions into an implementation sequence for the final-year project.

## Phase 1: Vertical Skeleton

- Create the monorepo service layout: `backend/`, `frontend/`, `document/`, `ocr/`, deployment files, and docs.
- Add Compose-compatible Podman-first deployment with `frontend`, `backend`, `document`, and optional fake provider modes.
- Serve compiled frontend assets through Caddy and proxy `/api/*` to the backend.
- Expose backend health and dependency readiness endpoints.
- Add fake document extraction and fake OpenAI-compatible model providers so the whole stack runs without real parsers or models.

## Phase 2: Backend Foundation

- Build the Go backend with `net/http`, a lightweight router, SQLite, migrations, and `sqlc`.
- Implement first-run admin setup, HTTP-only cookie sessions, admin/member roles, and structured API errors.
- Add provider configuration using environment defaults plus backend-side admin settings.
- Add activity history, debug mode controls, lightweight observability, and backend-owned mounted-volume state.

## Phase 3: Knowledge Bases And Documents

- Implement private/public knowledge bases, admin-controlled public visibility, and scoped permissions.
- Implement document upload, immediate privacy-scoped duplicate warning, rename, delete, move, reprocess, preview, and original download.
- Store uploaded and generated files using internal IDs and document versions.
- Use background ingestion jobs with status, retries, cancellation, resource limits, and tombstone deletion.
- Keep document versions internal while showing useful status and failure details.

## Phase 4: Document Processing

- Implement the Python `document` service as a mostly stateless HTTP extraction service.
- Use one Markdown-normalizing parser path for supported office inputs: PDF, DOCX, XLSX, PPTX, and common images.
- Return versioned extraction packages inline with normalized Markdown, metadata, warnings, OCR status, and best-available source anchors.
- Add conditional OCR fallback through an isolated replaceable `ocr` service.
- Preserve tables as readable Markdown and support image text retrieval without general vision-language reasoning.

## Phase 5: Indexing And Retrieval

- Implement pluggable backend chunking strategies with structure-aware Markdown chunking as the baseline.
- Store canonical extracted Markdown per document version and chunk/source-anchor metadata in SQLite.
- Use `chromem-go` for vector search, with SQLite authoritative and the vector index treated as rebuildable derived state.
- Store embedding vectors only in `chromem-go`; track embedding model identity and mark indexes stale after model changes.
- Add SQLite FTS for document management search and possible hybrid retrieval.
- Defer reranking until vector and hybrid retrieval are working and measured.

## Phase 6: Agentic Chat

- Embed `github.com/nep-0/harness` inside the backend application for streamed OpenAI-compatible tool execution.
- Persist the canonical harness transcript in SQLite, including assistant tool calls and tool results, while keeping internal tool messages out of ordinary UI responses.
- Use one OpenAI-compatible model API path for cloud and local providers.
- Implement multi-turn chat where retrieval is a constrained backend tool.
- Require retrieval for knowledge-base answers, enforce backend-controlled tool scope, and persist citation evidence.
- Stream chat responses with lightweight progress events and user-facing cancellation.
- Show unsupported answers when retrieved evidence is weak or missing.

## Phase 7: Frontend Workbench

- Build the frontend with React, Vite, TypeScript, Tailwind CSS, and accessible UI primitives.
- Implement a quiet English-first workbench UI with Chinese locale support.
- Include login/setup, knowledge-base management, document management, chat with citations, and compact admin/settings areas.
- Use handwritten typed fetch wrappers, frontend-owned localization, and citations-first evidence display.
- Open citation previews focused on extracted source anchors rather than full original document rendering.

## Phase 8: Local Model Profile

- Add optional local-model Compose profile for `llm` and `embedding` llama.cpp services.
- Mount GGUF model files from a host model directory rather than committing or baking them into images.
- Keep exact model names deferred until testing; document final tested models in the evaluation report.
- Add readiness/status handling for local model loading and ordinary CPU deployment constraints.

## Phase 9: Evaluation And Report

- Build a small controlled mixed office evaluation dataset with owned labels/questions in the repo and large documents stored outside git.
- Implement a scripted evaluation runner for ingestion, question answering, retrieval evidence, citation correctness, latency, and resource metrics.
- Focus on two primary comparisons: chunking strategy and retrieval strategy.
- Run a documented offline smoke test with local models, cloud settings disabled, and internet disconnected or blocked.
- Write lightweight third-party attribution and final runbook documentation.
