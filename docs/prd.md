# PRD: Lightweight Private Multimodal Intelligent Office Assistant

## Problem Statement

Small and micro enterprises need practical AI-assisted office search and question answering, but public cloud AI tools can create data-security concerns and traditional private AI deployments are often too expensive or too complex. The user needs a final-year project that demonstrates a local-first, CPU-only, document-grounded office assistant that can import common documents, build a private knowledge base, support multi-round question answering, show citations, and be deployed with Docker Compose.

The project must remain realistic for an undergraduate graduation timeline. It should focus on a coherent private RAG system rather than expanding into a full OA platform, enterprise identity system, or general autonomous-agent product.

## Solution

Build an English-first, local-first private intelligent office assistant for small and micro enterprises. The system will use a Go backend, SQLite metadata storage, `chromem-go` local vector indexing, sidecar document processing services, configurable model providers, and a practical SPA frontend served through Caddy.

The required product experience is:

- Admin logs in, configures model providers, creates users, creates knowledge bases, and assigns users to knowledge bases.
- Users upload native text office documents, scanned or image-based documents, and documents with embedded images.
- The system calls a MarkItDown document-processing service over HTTP, which decides whether OCR is needed and may call an OCR service such as MinerU.
- The backend stores original files, metadata, chunks, embeddings, chat sessions, chat messages, and citation snapshots locally.
- Users ask multi-round questions inside one knowledge base, receive streamed answers over SSE, and each answer includes citations.
- The system supports a fully offline local path using local model providers, while admins may optionally enable cloud embedding or chat providers.
- Docker Compose starts the backend, Caddy frontend, MarkItDown service, OCR service, local LLM service, local embedding service, and persistent volumes.
- Evaluation proves CPU-only viability, retrieval quality, citation quality, and performance on an office-like dataset.

## User Stories

1. As an admin, I want to log in with a local account, so that I can manage the private office assistant.
2. As a user, I want to log in with a local account, so that I can access assigned knowledge bases securely.
3. As an admin, I want to create user accounts, so that employees can use the system.
4. As an admin, I want to edit or disable user accounts, so that access can be managed locally.
5. As an admin, I want to assign users to knowledge bases, so that users only see relevant document collections.
6. As an admin, I want to create knowledge bases, so that documents can be grouped by topic or office need.
7. As an admin, I want to delete knowledge bases with confirmation, so that obsolete collections and their derived data can be removed.
8. As a user, I want to see only assigned knowledge bases, so that I do not access unrelated company information.
9. As an admin, I want to see all knowledge bases, so that I can manage the system.
10. As a user, I want to upload documents to assigned knowledge bases, so that I can make them searchable.
11. As a user, I want to upload native text PDFs, so that ordinary reports can be used for Q&A.
12. As a user, I want to upload common office documents, so that Word, PowerPoint, Excel-like, and similar files can be imported when supported by the processor.
13. As a user, I want to upload scanned or image-based documents, so that OCR-based content can be included.
14. As a user, I want documents with embedded images to be imported, so that image-containing office files are not rejected.
15. As a user, I want unsupported file types to show clear errors, so that I know why import failed.
16. As a user, I want large uploads to show clear limit errors, so that the system does not hang on CPU-only hardware.
17. As a user, I want to see document processing status, so that I know whether a file is pending, processing, indexed, or failed.
18. As a user, I want failed documents to show failure reasons, so that I can decide whether to retry or replace them.
19. As a user, I want duplicate uploads to be detected by file hash, so that I do not repeatedly index the same file by mistake.
20. As a user, I want same-name but different-content files to be allowed, so that real office files with repeated names can still be stored.
21. As a user, I want to rename my uploaded documents, so that document lists stay understandable.
22. As a user, I want to delete my own uploaded documents, so that incorrect or stale documents can be removed.
23. As an admin, I want to delete any document, so that the knowledge base can be maintained.
24. As a user, I want to download original uploaded files, so that I can verify cited material directly.
25. As a user, I want document deletion to remove original files, chunks, and vectors, so that stale content is not retrieved.
26. As a user, I want document replacement to reprocess and reindex the document, so that updated content is used for future answers.
27. As a user, I want old chat history to remain after document replacement, so that past conversations are not silently erased.
28. As a user, I want old citations to remain understandable after source changes, so that historical answers can still be reviewed.
29. As a user, I want stale citations to be marked when sources are deleted or replaced, so that I do not mistake historical citations for current sources.
30. As a user, I want to ask questions within one knowledge base, so that answers are grounded in the selected collection.
31. As a user, I want each chat session to belong to exactly one knowledge base, so that retrieval and citations stay clear.
32. As a user, I want multi-round conversation, so that I can ask follow-up questions naturally.
33. As a user, I want every answer in a multi-round conversation to include citations, so that I can verify each round.
34. As a user, I want factual follow-up questions to retrieve fresh supporting chunks, so that the model does not rely only on chat history.
35. As a user, I want streamed answer text, so that the interface feels responsive while generation is running.
36. As a user, I want citations to appear with the completed streamed answer, so that sources are attached to the final response.
37. As a user, I want the assistant to admit when no good source is found, so that it does not fabricate document facts.
38. As a user, I want possibly related sources to be shown only when they pass a threshold, so that weak retrieval is not presented as evidence.
39. As a user, I want citations to show document name, page number when available, preview text, upload time, and retrieval score when available, so that I can evaluate the source.
40. As a user, I want OCR-based citations to show OCR preview and file/page references, so that scanned documents are still traceable.
41. As an admin, I want to configure embedding providers, so that the system can use local or cloud embeddings.
42. As an admin, I want to configure chat providers, so that the system can use local or cloud LLMs.
43. As an admin, I want provider settings stored locally, so that configuration survives restart.
44. As an admin, I want provider API keys to be masked after saving, so that raw keys are not exposed in normal UI/API responses.
45. As an admin, I want to test provider connectivity, so that model configuration problems are visible.
46. As an admin, I want cloud providers to be optional, so that the system can remain fully offline when required.
47. As an admin, I want local providers to use OpenAI-compatible interfaces where possible, so that cloud and local providers can share integration logic.
48. As an admin, I want embedding-provider changes to mark affected chunks stale, so that mixed embedding spaces are not queried incorrectly.
49. As an admin, I want to trigger re-embedding for stale chunks, so that indexes can be brought back in sync.
50. As an admin, I want chat provider changes to take effect immediately, so that generation can be switched without rebuilding vectors.
51. As an admin, I want to see backend health, so that I can tell whether the core service is running.
52. As an admin, I want to see database status, so that local persistence problems are visible.
53. As an admin, I want to see document processor and OCR status, so that import failures can be diagnosed.
54. As an admin, I want to see embedding and chat provider status, so that model-service problems can be diagnosed.
55. As an admin, I want to see queued and processing document jobs, so that import backlog is visible.
56. As an admin, I want CPU-safety limits to be configurable, so that the system can protect weak local hardware.
57. As an admin, I want upload size, page count, import concurrency, queue size, context chunks, and answer timeout limits, so that the system fails predictably under load.
58. As a deployer, I want one Docker Compose command to start the system, so that local installation is simple.
59. As a deployer, I want Caddy to serve the frontend and reverse proxy API requests, so that the browser has one HTTP entry point.
60. As a deployer, I want persistent volumes for uploads, SQLite, and vector indexes, so that data survives container restarts.
61. As a developer, I want a Go backend, so that orchestration, API behavior, indexing, and deployment use one main implementation language.
62. As a developer, I want document conversion and OCR isolated in sidecar containers, so that specialized tools do not dominate the Go backend.
63. As a developer, I want an in-process document job queue for v1, so that document processing is simple without Redis or external queues.
64. As a developer, I want pending or processing jobs recovered on restart, so that uploads are not orphaned.
65. As a developer, I want structured logs without raw document content, full prompts, API keys, or full answers, so that debugging does not leak private content.
66. As an evaluator, I want import and indexing metrics, so that document-processing performance is measurable.
67. As an evaluator, I want storage and runtime metrics, so that CPU-only deployment costs are visible.
68. As an evaluator, I want retrieval quality metrics, so that vector-only retrieval can be compared with optional hybrid or reranking variants.
69. As an evaluator, I want citation quality metrics, so that source traceability is measured rather than assumed.
70. As an evaluator, I want user-facing latency metrics, so that retrieval, generation, and total response time are clear.
71. As an evaluator, I want evaluation results to state provider configuration, so that local and cloud results are not confused.
72. As an evaluator, I want a prepared question-set harness, so that thesis tables can be generated consistently.
73. As a project author, I want English-first UI, prompts, docs, and reports, so that the project presentation is consistent.
74. As a project author, I want autonomous agent features to be stretch-only, so that the core RAG system is completed first.
75. As a project author, I want a minimum successful demo path, so that graduation evaluation can focus on a complete working system.

## Implementation Decisions

- The official title is “Lightweight Private Multimodal Intelligent Office Assistant for Small and Micro Enterprises.”
- The internal subtitle is “A CPU-only local RAG system with document-grounded multi-round Q&A and citation traceability.”
- The system is English-first for UI, prompts, documentation, and evaluation reports.
- The core product is a fully offline-capable local RAG system. Cloud model providers may be admin-enabled optional providers but must not be required for the offline workflow, final demo, or offline evaluation claim.
- Go is the main implementation language.
- SQLite stores application metadata.
- `chromem-go` provides embedded local vector indexing.
- Original uploaded files are kept in local storage after chunking and indexing.
- Docker Compose is the deployment unit.
- The frontend is a React/TypeScript/Vite-style SPA.
- The frontend container uses Caddy to serve static assets and reverse proxy API requests to the Go backend.
- The UI should use a dense, practical admin-tool style with a sidebar, compact tables, status badges, upload progress, and citation panels.
- The app should not use a landing page, marketing hero, decorative dashboard, or promotional layout.
- Authentication uses local username/password accounts, secure password hashes, JWT API authentication, and two roles: `Admin` and `User`.
- Admins can manage users, create/delete knowledge bases, assign users to knowledge bases, configure providers, and delete any document.
- Users can access assigned knowledge bases, upload documents, rename/delete their own uploaded documents, download originals, ask questions, and view citations.
- Simple knowledge-base membership is the only v1 permission model beyond roles.
- Each chat session belongs to exactly one knowledge base.
- SQLite entities include users, knowledge bases, knowledge-base memberships, documents, chunks, chat sessions, chat messages, citations, and model provider settings.
- V1 avoids organizations, teams, document versions, audit logs, and complex permission tables.
- REST JSON APIs cover authentication, knowledge-base operations, document operations, chat session/message operations, admin user management, provider settings, and system health.
- Chat answers stream over SSE.
- SSE streams text tokens first and sends message metadata plus citations in the final event.
- GraphQL and WebSocket are out of v1 unless bidirectional realtime behavior becomes necessary later.
- Document conversion runs in a MarkItDown HTTP sidecar container.
- MarkItDown decides whether OCR is needed and may call an OCR sidecar service such as MinerU.
- The Go backend owns orchestration, persistence, indexing, retrieval, user-facing APIs, and provider calls.
- The document-processing service returns chunks with source metadata.
- Each returned chunk includes chunk ID, document ID, knowledge-base ID, content, source file name, page number when available, chunk index, content type, token or character count, and metadata for warnings or image references.
- The Go backend may generate IDs when needed, but source-location metadata is required for citations.
- The import pipeline supports native text office documents, scanned or image-based documents, and images embedded inside documents.
- Table-specific reasoning, chart understanding, pixel-level grounding, and full visual question answering are not required.
- Uploads compute file hashes.
- Same-user same-file same-knowledge-base duplicate uploads warn or skip by default.
- Same-name different-hash files are allowed and disambiguated by upload time or internal ID.
- Document processing uses an in-process Go job queue in v1.
- Document statuses include pending, processing, indexed, and failed.
- On restart, pending or processing jobs are made processable again by returning them to pending or marking them failed-retryable.
- V1 avoids Redis and external queues unless a real bottleneck appears.
- New documents are parsed, chunked, embedded, stored, and indexed.
- Modified documents replace the previous indexed version at document level.
- Deleted documents remove original files, metadata, chunks, and vectors.
- If reprocessing a modified document fails, the previous indexed version remains available until replacement succeeds.
- Knowledge-base deletion requires confirmation and hard-deletes documents, original files, chunks, vectors, chat sessions, messages, and citations.
- Deleting or reindexing a document does not delete chat history.
- Historical citations are marked source-deleted or source-replaced when underlying sources change.
- Citations store both references and snapshots: document/chunk IDs plus citation text preview and source metadata captured at answer time.
- Retrieval is vector-first with `chromem-go`.
- Reranking is optional and belongs to evaluation or optimization, not required v1 behavior.
- Keyword/hybrid retrieval is optional for experiments or fallback.
- Multi-round factual questions build a retrieval query from the current question plus recent turns or a short conversation summary.
- Factual answers must retrieve fresh chunks and cite the evidence used in that round.
- The model should not answer factual follow-ups only from chat history unless the answer is purely conversational.
- If retrieval finds insufficient evidence, the assistant says it cannot find enough supporting information and should not fabricate a factual answer.
- Possibly related sources may be shown only if they pass a loose threshold.
- The answer prompt requires the model to answer from retrieved context, cite supporting chunks, admit insufficient context, keep business-office answers concise, and avoid inventing document facts from general knowledge.
- Admins configure two model provider slots: embedding provider and chat provider.
- Each provider slot can be local or cloud.
- Provider settings are stored in SQLite.
- API keys may be stored in SQLite for v1 local deployment convenience.
- API responses must not return raw provider API keys after saving.
- The settings UI shows masked provider keys and allows replacement.
- The first provider contract uses OpenAI-compatible chat and embedding API shapes.
- Local offline providers are expected to use llama.cpp-compatible containers for chat and embeddings.
- Embedding chunks store provider ID/name, model name, vector dimension, and embedding status.
- When the active embedding provider changes, affected chunks are marked stale or needing re-embedding.
- Retrieval uses only chunks embedded with the active provider/model.
- Admins can trigger re-embedding per knowledge base or globally.
- Chat provider changes can take effect immediately because they do not invalidate vectors.
- Required Docker Compose services are backend, frontend/Caddy, MarkItDown, OCR, LLM, embedding, and persistent volumes for uploads, SQLite, and vector index data.
- CPU-only deployment is defined as no GPU on commodity office hardware.
- Minimum target is 8 CPU cores and 16 GB RAM.
- Recommended target is 12-16 CPU cores and 32 GB RAM.
- The major project risk is CPU/RAM being too weak to run local chat, local embedding, OCR, document processing, backend, frontend, SQLite, and vector index together.
- A local model proof must measure RAM usage, startup time, embedding latency, first-token latency, and total generation time.
- If local generation is slow, the demo still shows the local path working on a small prompt/document and may separately show cloud chat as an admin-enabled performance mode.
- Evaluation must separate offline viability from interactive performance.
- System health exposes backend status, database status/path, document processor status, OCR status, embedding provider status, chat provider status, active provider types, model names when available, vector index stats, queued/processing jobs, and disk usage when easy.
- Structured backend logs include timings, IDs, statuses, provider names, and error categories.
- Logs do not include raw document content, full prompts, API keys, or full answers by default.
- Configurable CPU-safety limits include max upload size, max pages per document, max concurrent imports, max queued jobs, max chat context chunks, and answer timeout.
- The first technical spike includes a Go HTTP server, SQLite connection, provider interfaces, one cloud embedding provider, one cloud chat provider, fake/static chunk input, and SSE streaming response.
- The second technical spike integrates MarkItDown HTTP chunking.
- Milestone 1 may use cloud model providers, but later milestones must add local llama.cpp-compatible providers and prove the offline path.

## Testing Decisions

- Prefer the highest practical behavioral seam: HTTP-level backend tests for API behavior and SSE chat behavior.
- Test external behavior rather than internal implementation details.
- Use fake HTTP services for MarkItDown, embedding providers, and chat providers so tests are deterministic and do not require real models.
- Primary backend API seam should cover authentication, knowledge-base membership, document lifecycle, provider settings, chat submission, SSE final citation event, health responses, and permission boundaries.
- Document-processing seam should test the backend against a fake MarkItDown HTTP service that returns known chunks with source metadata.
- Model-provider seam should test against fake OpenAI-compatible embedding and chat providers.
- Provider tests should cover local/cloud configuration behavior, masked API key readback, provider connectivity checks, provider switching, stale chunk marking, and re-embedding triggers.
- Retrieval tests should seed known chunks and verify retrieval filtering by active provider/model, knowledge base, membership, and document state.
- Chat tests should verify multi-round behavior, fresh retrieval for factual follow-ups, final SSE citation event, no-source behavior, and citation snapshots.
- Document lifecycle tests should cover upload, duplicate detection, status transitions, restart recovery, replacement, deletion, failed reprocess preserving old indexed version, original-file retention, and original-file download.
- Permission tests should cover admin capabilities, assigned-user access, unassigned-user denial, user-owned document rename/delete, and admin delete-any-document behavior.
- Knowledge-base deletion tests should verify hard deletion of documents, files, chunks, vectors, chat sessions, messages, and citations.
- Health tests should verify service/provider status reporting and queued/processing job visibility.
- Logging tests should focus on ensuring sensitive fields are not included in structured logs where practical.
- Evaluation harness seam should run prepared questions against a seeded knowledge base and export provider configuration, machine specs, top-k, retrieved/cited chunks, latency, and manual judgment fields.
- Frontend tests should focus on visible workflows where feasible: login, knowledge-base navigation, document upload/status/errors, chat streaming, citation display, provider settings, health page, and role-specific UI behavior.
- Docker Compose smoke tests should verify that the required services start, the Caddy entry point serves the frontend, API proxying reaches the backend, and persistent volumes are mounted.
- The first technical spike acceptance test should prove provider abstraction and SSE streaming using fake/static chunks before document upload is wired.
- Milestone 1 acceptance should prove login, knowledge-base creation, PDF upload, MarkItDown chunking, SQLite metadata/chunk storage, configured embedding provider use, `chromem-go` indexing, configured chat provider use, multi-round chat, and at least one citation per answer.

## Out of Scope

- Full OA workflows.
- Workflow center.
- Email integration.
- Calendar integration.
- Multi-company tenancy.
- Fine-grained permissions beyond knowledge-base membership.
- Enterprise identity integrations such as SSO, OAuth, or directory sync.
- Password reset email.
- Real-time collaboration.
- Complex analytics UI.
- Marketing landing page or promotional dashboard.
- Separate Postgres, Redis, object storage, or distributed queues unless a real bottleneck appears.
- Table-specific reasoning as a core requirement.
- Chart reasoning.
- Pixel-level visual grounding.
- Full visual question answering.
- GPU-only features.
- Cloud-required features.
- Broad claims about general answer accuracy without narrowly defined scoring.
- Full duplicate-management UI.
- Soft-delete or recovery behavior in v1.
- Per-user model-provider selection.
- GraphQL.
- WebSocket streaming unless bidirectional realtime behavior becomes necessary later.
- Arbitrary autonomous agents.
- Arbitrary shell access, browser automation, workflow execution, or multi-step business process automation.

## Further Notes

- Autonomous agent features are stretch-only and may be explored after the core offline RAG system, Docker deployment, role management, citations, error handling, and evaluation are complete.
- If implemented, the stretch agent should be a narrow document task assistant with predefined tools such as summarizing a selected document, comparing two documents, extracting action items, drafting a short report from cited sources, or answering follow-up questions within one knowledge base.
- The dataset should mix a reproducible public subset, 30-100 self-built office-like documents, and a stress subset starting at 100 documents. A 1,000-document stress test should only be attempted if the 100-document run time is reasonable.
- Finance datasets may be supplementary, but they should not replace office-like test files because the project is an office assistant rather than a finance QA system.
- Primary thesis contribution is CPU-only private RAG optimization for office documents.
- Primary experiments are chunking/indexing strategy comparisons and retrieval strategy comparisons.
- Model quantization is supporting infrastructure unless there is enough time to run a clean model comparison.
- Evaluation metrics cover import/indexing, storage/runtime, retrieval quality, citation quality, and user-facing latency.
- Evaluation results must state whether they used fully local embedding/chat, local embedding with cloud chat, cloud embedding with local chat, or fully cloud providers.
- The minimum successful demo is: Docker Compose starts the system; admin logs in; admin creates a knowledge base and user; user uploads a native PDF and an image/scanned document; system indexes them; user asks at least two questions; every answer has citations; user deletes or replaces a document; system updates the index; health page shows local service status; evaluation report includes timing and citation metrics.

