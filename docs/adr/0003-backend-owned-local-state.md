# Backend-Owned Local State

Persistent application state lives in the backend container's mounted volume. The backend uses pure-Go SQLite for relational state such as users, knowledge bases, documents, chunks, citations, ingestion jobs, and configuration, and uses `chromem-go` for the local vector index so the final deployment can stay lightweight and avoid a separate database service.

