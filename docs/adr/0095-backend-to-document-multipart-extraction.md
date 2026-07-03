# Backend To Document Multipart Extraction

The backend sends stored original files to the document service over HTTP multipart requests for extraction, and the document service returns an extraction package. This avoids shared-volume coupling between backend and document processing while preserving parser isolation.

