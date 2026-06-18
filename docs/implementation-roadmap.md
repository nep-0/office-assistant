# Implementation Roadmap

Build the system in dependency order so the complete RAG loop works before OCR, optimization, or polish.

## Milestones

1. Go backend skeleton: authentication, SQLite persistence, knowledge bases, and documents.
2. Minimal frontend: login, knowledge base pages, and document pages.
3. MarkItDown HTTP service integration returning chunks.
4. First thin RAG slice using model-provider interfaces, initially allowed to use cloud embedding and cloud LLM adapters for speed of development.
5. Local model proof with llama.cpp-compatible chat and embedding containers, measuring CPU/RAM viability.
6. `chromem-go` indexing and citation return path.
7. Replace or supplement cloud embedding and cloud LLM adapters with local llama.cpp-compatible services.
8. Docker Compose wiring for the required local services.
9. OCR integration through the document-processing service.
10. Admin/user polish, required error states, and status page.
11. Evaluation scripts and report data.

## First Technical Spike

Before building the full app, create a narrow spike:

- Go HTTP server.
- SQLite connection.
- Model provider interfaces.
- One cloud embedding provider implementation.
- One cloud chat provider implementation.
- Fake or static chunk input.
- SSE streaming response.

The purpose is to prove provider abstraction and streaming chat before wiring document upload.

The second spike should integrate MarkItDown HTTP chunking.

## Build Principle

Do not start with OCR or model optimization. First prove the end-to-end path: upload document, chunk document, index chunks, ask question, retrieve context, generate answer, and show citations. Cloud model adapters may remain as admin-enabled optional providers, but the final demo and evaluation must prove the fully offline local model path.

## Minimum Successful Demo

The demo is successful when:

- One Docker Compose command starts the system.
- An admin logs in.
- The admin creates a knowledge base and a user.
- The user uploads a native PDF and an image or scanned document.
- The system indexes the documents.
- The user asks at least two questions.
- Answers include citations for every round of the conversation.
- The user deletes or replaces a document.
- The system updates the index.
- The status page shows local service health.
- The evaluation report includes timing and citation metrics.

## Milestone 1 Acceptance Test

Backend plus minimal frontend can create and log in a user, create a knowledge base, upload one native text PDF, call MarkItDown over HTTP to get chunks, store document and chunk metadata in SQLite, call the configured embedding provider, index chunks with `chromem-go`, call the configured LLM provider, and support a multi-round chat where each answer returns at least one citation. In Milestone 1, the configured model providers may be cloud providers. Later milestones must add local llama.cpp-compatible providers and prove the offline path.
