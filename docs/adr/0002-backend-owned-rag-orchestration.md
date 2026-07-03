# Backend-Owned RAG Orchestration

The Go backend owns the RAG workflow orchestration, including upload lifecycle, knowledge base state, indexing decisions, retrieval, prompt assembly, model calls, citations, and user-visible status. The Python document service is limited to extracting normalized document content and metadata, with OCR delegated to the OCR service when needed, so product behavior does not become split across two backend applications.

