# Pluggable Backend Chunking Strategies

The backend exposes chunking as a strategy interface so multiple implementations can be compared during experiments. The recommended baseline is structure-aware Markdown chunking that splits by document structure when available, then applies size limits and overlap while preserving source anchors for traceable answers.

