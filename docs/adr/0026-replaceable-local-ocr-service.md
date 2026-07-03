# Replaceable Local OCR Service

The OCR service starts with a simple local OCR engine exposed over HTTP and remains replaceable. The document service owns OCR calls and normalization, while the backend depends only on extraction packages and does not embed OCR-engine-specific behavior.

