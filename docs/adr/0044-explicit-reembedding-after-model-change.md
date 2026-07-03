# Explicit Re-Embedding After Model Change

When the active embedding provider or model differs from indexed chunks, affected indexes are marked stale and require an explicit re-embedding or reprocess action. The system does not silently mix embeddings from different models in one retrieval path because mixed embedding spaces make retrieval quality unpredictable.

