# Replace Document Index on Modification

Incremental updates are handled at the document level. New documents are parsed, chunked, embedded, stored, and added to the vector index; modified documents replace the previous indexed version by document ID; deleted documents remove the file, metadata, chunks, and vectors.

## Consequences

The system will not attempt chunk-level diffing. If reprocessing a modified document fails, the previous indexed version should remain available until the new version is successfully processed.
