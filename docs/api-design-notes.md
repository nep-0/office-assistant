# API and Design Notes

This document records implementation-facing design decisions that are stable enough to guide the backend and frontend.

## V1 Data Model Boundary

SQLite should model these core entities:

- `users`
- `knowledge_bases`
- `knowledge_base_memberships`
- `documents`
- `chunks`
- `chat_sessions`
- `chat_messages`
- `citations`
- `model_provider_settings`

V1 should avoid adding organizations, teams, document versions, audit logs, or complex permission tables beyond simple knowledge-base membership.

## API Style

Use REST JSON APIs for normal operations:

- Authentication.
- Knowledge base create/list/delete.
- Document upload/list/rename/delete/status.
- Chat session create/list.
- Chat message list.
- Admin user management.
- Model provider settings.
- System health.

Use SSE for chat answer streaming. The client sends a normal request to submit a user message, then receives streamed answer events. Text tokens should stream first; the final stream event should include message metadata and citations for that answer.

V1 should avoid GraphQL and WebSocket unless bidirectional real-time behavior becomes necessary later.

Initial implemented endpoints:

- `POST /api/auth/bootstrap`: create the first admin when no users exist.
- `POST /api/auth/login`: exchange username/password for a JWT.
- `GET /api/me`: return the authenticated user.
- `GET /api/admin/users`: list users; admin only.
- `POST /api/admin/users`: create users; admin only.
- `GET /api/knowledge-bases`: list visible knowledge bases.
- `POST /api/knowledge-bases`: create a knowledge base; admin only.
- `POST /api/knowledge-bases/{id}/members`: assign a user to a knowledge base; admin only.
- `GET /api/knowledge-bases/{id}/documents`: list documents in an accessible knowledge base.
- `POST /api/knowledge-bases/{id}/documents`: upload a document to an accessible knowledge base and queue processing.
- `GET /api/provider-settings`: read masked provider settings; admin only.
- `PUT /api/provider-settings`: update provider settings; admin only.
- `POST /api/chat/stream`: stream a chat answer with final citations; authenticated users.

## Model Provider Settings

Admins configure two provider slots:

- Embedding provider.
- Chat provider.

Each slot can be configured as `local` or `cloud`, with provider settings stored in SQLite. The status/settings page should allow admins to test provider connectivity.

V1 should not support per-user provider selection.

The first provider contract should use Eino's OpenAI-compatible chat and embedding components, so cloud providers and local llama.cpp-compatible servers can share the same integration path while the application keeps narrow provider interfaces of its own.

Provider API keys may be stored in SQLite for v1 local deployment convenience. API responses should not return raw keys after saving; the settings UI should show masked values and allow replacement. This should be documented as local configuration storage, not enterprise-grade secret management.

## Runtime Configuration

The backend should load runtime configuration from a JSON file. The path can be passed with `-config` or through `OA_CONFIG`; if no path is provided, defaults are used for local development.

The example file lives at `config/example.json` and covers HTTP bind address, SQLite path, upload directory, MarkItDown service URL, JWT secret, and log level.

## Embedding Provider Changes

Chunks should store embedding metadata such as provider ID/name, model name, vector dimension, and embedding status. When the active embedding provider changes, existing chunks should be marked stale or needing re-embedding rather than marking only the whole knowledge base.

Retrieval should use only chunks embedded with the active provider/model. Admins can trigger re-embedding for stale chunks per knowledge base or globally.

Chat provider changes can take effect immediately because they do not invalidate stored vectors.

## Document Processing Jobs

Use a simple in-process job queue in the Go backend for v1. Uploading a document creates a document record and queues processing.

Document status should move through:

- `pending`
- `processing`
- `indexed`
- `failed`

The backend should process one or a small configurable number of jobs concurrently. The frontend should poll document status rather than requiring a separate queue or realtime job system.

V1 should avoid Redis or an external queue unless a real bottleneck appears.

On backend startup, documents left in `pending` or `processing` should be made processable again, either by returning them to `pending` or marking them `failed_retryable`. V1 should not implement complex durable job leasing.

## File Retention

Keep original uploaded files in local storage after chunking and indexing. Originals are needed for citation review, reprocessing, replacement, and user download or inspection.

Deleting a document should delete the original file and derived data, including chunks and vectors.

Users should be able to download original files from knowledge bases they can access. Admins can access all documents.

Users can upload documents to assigned knowledge bases. Users can rename or delete only their own uploaded documents. Admins can delete any document.

Deleting a knowledge base requires confirmation and should hard-delete its documents, original files, chunks, vectors, chat sessions, messages, and citations. V1 does not require soft-delete or recovery behavior.

Uploads should compute a file hash. If the same user uploads the same file into the same knowledge base, the system should warn or skip by default. Files with the same name but different hashes may be allowed and disambiguated by upload time or internal ID.

Deleting or reindexing a document should not automatically delete chat history. Existing messages and citations should remain as historical records, with citations marked `source_deleted` or `source_replaced` when the underlying document or chunk no longer exists. New answers should retrieve only from current indexed chunks.

Citations should store both references and snapshots: document/chunk IDs for navigation, plus citation text preview and source metadata captured at answer time. Historical answers should remain understandable even if the source document is later deleted or replaced.

## No-Source Behavior

If retrieval does not find enough supporting information in the selected knowledge base, the assistant should say that it cannot find enough evidence and should not fabricate a factual answer. The UI may show closest retrieved chunks as possibly related sources only if they pass a loose threshold.

Every factual answer should be grounded by citations.

## Retrieval Tool

Retrieval should be implemented as a model-callable tool, not as hidden HTTP handler logic or a forced pre-generation step. The tool input includes knowledge base ID, query text, optional recent messages, and top-k. The tool output is a list of chunks with source metadata for prompt grounding and citation snapshots.

The current spike uses a deterministic static retrieval tool triggered by the mock provider's tool-call event. The production implementation should replace it with a `chromem-go` backed retrieval tool and expose the same capability through Eino tool-calling so the LLM can request retrieval explicitly. If the model does not request retrieval, no retrieval should run and no citations should be fabricated.

## Answer Prompt Policy

The answer prompt should require the model to:

- Answer from retrieved context.
- Cite supporting chunks.
- Admit when context is insufficient.
- Keep business-office answers concise.
- Avoid inventing document facts from general knowledge.

## System Health

The status page/API should expose:

- Backend status.
- Database path/status.
- Document processor status.
- OCR status.
- Embedding provider status.
- Chat provider status.
- Active provider types.
- Model names when available.
- Vector index stats.
- Queued and processing document jobs.
- Disk usage for uploads and indexes when easy to measure.

## Logging Policy

Use structured backend logs for uploads, processing jobs, provider calls, retrieval, chat generation, and errors. Logs should include timings, IDs, statuses, provider names, and error categories.

Do not log raw document content, full prompts, API keys, or full answers by default.
