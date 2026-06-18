# Project Scope

This final-year project is a lightweight private multimodal intelligent office assistant for small and micro enterprises. The system prioritizes fully offline local deployment, document-based Q&A, citation traceability, and CPU-only operation.

Official title: Lightweight Private Multimodal Intelligent Office Assistant for Small and Micro Enterprises.

Internal subtitle: A CPU-only local RAG system with document-grounded multi-round Q&A and citation traceability.

## Required Frontend Pages

- Login.
- Knowledge base list.
- Document list and upload.
- Multi-round chat/Q&A with citations on every answer.
- Admin user management.
- Basic system/status page for model and service health.

## Frontend Stack

Use a simple SPA frontend, likely React, TypeScript, and Vite, communicating with the Go backend API.

## Frontend Design Direction

Use a dense, practical admin-tool style: left sidebar for knowledge bases and settings, main document/chat workspace, restrained colors, compact tables, status badges, upload progress, and citation panels.

The app should not use a landing page, marketing hero, decorative dashboard, or promotional layout.

## Authentication and Roles

Use local username/password accounts stored in SQLite, with securely hashed passwords and JWT-based API authentication. The system has two roles: `Admin` and `User`.

Admins can manage users, create or delete knowledge bases, assign users to knowledge bases, and delete any document. Users can upload documents to assigned knowledge bases, view documents, rename or delete their own uploaded documents, ask questions, and view citations within assigned knowledge bases.

## Deployment Services

Docker Compose should include:

- `backend`: Go API service.
- `frontend`: Caddy-based web container serving the frontend static assets and reverse proxying API requests to the Go backend.
- `markitdown`: document conversion and chunking service.
- `ocr`: OCR service, such as MinerU, called by the document-processing service when needed.
- `llm`: llama.cpp-compatible generation service.
- `embedding`: llama.cpp-compatible embedding service.
- Persistent volumes for uploaded files, SQLite data, and vector index data.

## Required Error States

The UI and backend should explicitly represent:

- Unsupported file type.
- Upload too large.
- Document conversion failed.
- OCR failed.
- Embedding service unavailable.
- LLM service unavailable.
- Indexing failed.
- No relevant sources found.
- Answer generation timeout.

Failed documents should show a status and reason in the document list.

## CPU-Safety Limits

The system should expose configurable limits for:

- Maximum upload size.
- Maximum pages per document.
- Maximum concurrent imports.
- Maximum queued import jobs.
- Maximum chat context chunks.
- Answer generation timeout.

The default behavior should protect CPU-only deployment, such as allowing one import job at a time, using a small retrieval context, and returning visible timeout errors instead of hanging requests.

## Privacy Claim

The system should be described as local-first private deployment. Uploaded files, extracted chunks, indexes, chat history, and user data are stored locally.

If an admin enables cloud model providers, relevant query or context data may be sent to that provider. The UI/settings and documentation should make this explicit. The project should not claim absolute confidentiality when cloud providers are enabled.

## Language Policy

The UI, documentation, default prompts, and evaluation report should be English-first. The system may handle Chinese documents or questions if the selected models support them, but Chinese support is not a primary acceptance requirement. Answers should normally follow the user's question language.

## Excluded Frontend Scope

- Full OA dashboard.
- Workflow center.
- Notifications.
- Complex analytics UI.
- SSO, OAuth, enterprise directory integration, password reset email, or a multi-tenant organization model.
- Separate Postgres, Redis, object storage, or distributed queues unless a real bottleneck appears.

## Non-Goals

- Full OA workflows.
- Email or calendar integration.
- Multi-company tenancy.
- Fine-grained permissions.
- Real-time collaboration.
- Chart reasoning.
- Pixel-level visual grounding.
- GPU-only features.
- Cloud-required features.

## Stretch Goals

- Autonomous agent features may be explored only after the core offline RAG system, Docker deployment, role management, citations, error handling, and evaluation are complete.
- If implemented, the autonomous agent should be a narrow document task assistant with predefined tools such as summarizing a selected document, comparing two documents, extracting action items, drafting a short report from cited sources, or answering follow-up questions within one knowledge base.
- The stretch agent should not have arbitrary shell access, browser automation, workflow execution, or multi-step business process automation.
