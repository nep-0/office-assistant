# Ingestion Retry Policy

Background ingestion jobs use limited automatic retries for transient dependency failures such as unavailable document, OCR, or embedding services. Content failures such as unsupported files, corrupt documents, or unusable extraction results are marked failed with details and require manual reprocess after settings or services change.

