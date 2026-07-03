# Pluggable Retrieval Strategies

The baseline retrieval path uses vector search over embeddings stored with `chromem-go`. Retrieval remains strategy-based so experiments can compare vector-only retrieval against hybrid keyword plus vector retrieval, with reranking added only if time and evaluation results justify the extra complexity.

