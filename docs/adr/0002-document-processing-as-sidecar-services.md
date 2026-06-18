# Document Processing as Sidecar Services

Document conversion will run in a separate MarkItDown container, while the Go backend calls it over HTTP. The MarkItDown service owns the decision of whether OCR is needed and may call an OCR service such as MinerU from its own container boundary, without embedding those runtimes directly into the main backend container.

## Consequences

The Go backend owns orchestration, persistence, indexing, retrieval, and user-facing APIs. The document-processing service must expose a stable HTTP interface and return chunks with enough metadata for indexing, citation display, and incremental re-indexing, so the rest of the system is not coupled to MarkItDown or a specific OCR engine.

Each returned chunk must provide `chunk_id`, `document_id`, `knowledge_base_id`, `content`, `source_file_name`, `page_number` when available, `chunk_index`, `content_type`, `token_or_char_count`, and `metadata` for parser warnings or image references. The Go backend may generate IDs when needed, but source-location metadata is required for citations.
