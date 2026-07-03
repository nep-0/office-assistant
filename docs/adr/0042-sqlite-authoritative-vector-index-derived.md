# SQLite Authoritative, Vector Index Derived

SQLite is the authoritative store for document, chunk, citation, and embedding metadata, while the `chromem-go` vector index is treated as rebuildable derived state. Ingestion writes vector entries, but if consistency is suspect the backend can rebuild or repair the vector index from SQLite-owned records and stored chunk content rather than relying on the vector index as the only source of identity.

