# Office Assistant

Domain language for the lightweight private multimodal intelligent office assistant. This glossary keeps project terms precise while the final-year project scope is refined.

## Language

**Fully Offline Core**:
The required operating mode where document parsing, OCR, embedding, indexing, retrieval, reranking, answer generation, metadata storage, and file storage all run locally without internet access. Cloud models may exist as admin-enabled optional providers, but they must not be required for the main offline workflow, demo, or evaluation.
_Avoid_: Cloud-first workflow, hybrid-by-default workflow

**Multimodal Document Import**:
The required import capability covering native text office documents, scanned or image-based documents, and images embedded inside documents. It does not require table-specific reasoning, chart understanding, or full visual question answering as core features.
_Avoid_: Full vision-language understanding, universal document understanding

**Knowledge Base**:
A named collection of imported documents that can be indexed and queried as a unit. Knowledge bases are created and deleted by admins, and normal users access them through membership assigned by admins.
_Avoid_: Folder, workspace, database

**Document Management**:
The required workflow for uploading, listing, renaming, deleting, assigning, and re-indexing imported documents. It excludes enterprise document features such as approval workflows, version history, and fine-grained access control.
_Avoid_: OA document workflow, enterprise content management

**Admin**:
A user role that can manage users, create or delete knowledge bases, and assign users to knowledge bases. Admins also retain ordinary document and question-answering capabilities.
_Avoid_: Superuser, owner

**User**:
A basic role for day-to-day office use of the system. Users can upload documents, view document lists, rename or delete their own uploaded documents, ask questions, and view citations within assigned knowledge bases, but cannot manage users or create/delete knowledge bases.
_Avoid_: Guest, member

**Citation**:
The source information shown with an answer so the user can verify it, including document name, page number when available, chunk or paragraph preview, upload time, and retrieval score when available. For images or scanned pages, citations use OCR text preview and file/page references rather than pixel-level bounding boxes.
_Avoid_: Source link only, bounding-box citation

**Chat Session**:
A multi-round conversation between a user and exactly one knowledge base. Cross-knowledge-base conversations are outside the required scope.
_Avoid_: Global chat, all-knowledge-base chat
