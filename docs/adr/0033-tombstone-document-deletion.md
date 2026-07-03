# Tombstone Document Deletion

Document deletion first marks the document as removed from user-visible retrieval scope, then the backend cleans up chunks, embeddings, extracted Markdown, and original files. This tombstone-style flow avoids stale deleted content appearing in answers even though SQLite, filesystem files, and vector indexes cannot be updated atomically as one transaction.

