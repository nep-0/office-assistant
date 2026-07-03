# Embedding Vectors Only In chromem-go

Generated embedding vectors are stored only inside the `chromem-go` vector index, while SQLite stores chunk identity, chunk content, embedding model identity, dimensions, hashes, and indexing status. If the vector index is lost or invalidated, the backend rebuilds it by re-embedding stored chunks rather than restoring vectors from SQLite.

