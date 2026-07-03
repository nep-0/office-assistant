# Office Assistant

This context defines the domain language for a lightweight private multimodal intelligent office assistant for small and micro enterprises.

## Language

**Dual-Mode Model Provider**:
The system can use either cloud-hosted OpenAI-compatible models during development and testing, or local model services for the final private deployment proof.
_Avoid_: Cloud-first system, offline-only system

**Extraction Package**:
A deterministic result produced by the document service from an uploaded office file, containing normalized Markdown, document structure, metadata, OCR status, warnings, and source anchors.
_Avoid_: Parsed chunks, indexed document

**Source Anchor**:
A stable reference to where extracted content came from in the original document, such as a page, sheet, slide, table, image, or region when available.
_Avoid_: Chunk ID, citation text

**OCR Fallback**:
Conditional text extraction used only when native document extraction is unavailable or too poor for a page, image, or region.
_Avoid_: OCR-first extraction, universal OCR

**Supported Office Input**:
A file type accepted for reliable ingestion in the final demo: PDF, DOCX, XLSX, PPTX, and common image formats.
_Avoid_: All office files, legacy Office formats

**Traceable Answer**:
An answer that cites supporting sources at document and page, sheet, or slide level, with a retrieved chunk preview for user verification.
_Avoid_: Exact visual citation, coordinate-level citation

**Knowledge Base**:
A document collection that scopes retrieval for questions and owns its indexed document set, with private or public visibility.
_Avoid_: Tag, folder, global library

**Private Knowledge Base**:
A knowledge base visible to its owner and admins.
_Avoid_: Personal folder

**Public Knowledge Base**:
A knowledge base visible to all signed-in users and admins.
_Avoid_: Shared drive, global index

**Admin**:
A local account role that can manage users, model provider settings, and all knowledge bases.
_Avoid_: Superuser, system owner

**Member**:
A local account role that can upload documents, manage their own knowledge bases, and ask questions.
_Avoid_: Employee, normal user

**Chunking Strategy**:
A backend-selected method for turning an extraction package into indexed chunks while preserving source anchors for later traceability.
_Avoid_: Parser chunking, document splitting

**Evaluation Dataset**:
A controlled set of public and constructed office documents with known questions, expected answers, and expected source locations for measuring the system.
_Avoid_: Generic QA benchmark, random document pile

**Document Lifecycle**:
The supported state changes for an uploaded document: ingest, rename, delete, reprocess, and move between knowledge bases.
_Avoid_: Document editing, content authoring

**Agentic Retrieval**:
A question-answering flow where the model can call a backend retrieval tool to search the selected knowledge base instead of receiving a fixed retrieval result for every turn.
_Avoid_: Automatic pre-retrieval, unconstrained tool use

**Quiet Office UI**:
A low-distraction interface optimized for managing knowledge bases, documents, and document-grounded chat workflows.
_Avoid_: Marketing UI, decorative dashboard

**Ordinary CPU Deployment**:
A local deployment target centered on a laptop or desktop CPU with about 16 GB RAM, with 8 GB treated as best-effort.
_Avoid_: GPU deployment, low-end guarantee

**Bilingual Office Content**:
Chinese and English office documents and questions, with Chinese support prioritized when trade-offs appear.
_Avoid_: English-only content, unrestricted multilingual support

**Bilingual UI**:
An English-first interface with a supported Chinese locale option.
_Avoid_: Chinese-only UI, unrestricted localization

**Table Retrieval**:
Answering questions from table content preserved during extraction, without treating spreadsheets as executable analytical models.
_Avoid_: Spreadsheet intelligence, formula engine

**Image Text Retrieval**:
Using OCR text and image references from documents without requiring general visual reasoning over charts, diagrams, screenshots, or photos.
_Avoid_: Vision-language reasoning, image understanding

**Unsupported Answer**:
A document-grounded response that states the selected knowledge base does not provide enough evidence, optionally showing closest retrieved sources.
_Avoid_: Hallucinated answer, uncited guess
