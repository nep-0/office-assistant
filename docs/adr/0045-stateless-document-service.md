# Stateless Document Service

The Python document service is mostly stateless across requests, using temporary working files only during extraction. Durable originals, extracted Markdown, ingestion status, and indexes remain in the backend-owned volume so the document service can be restarted or replaced without losing product data.

