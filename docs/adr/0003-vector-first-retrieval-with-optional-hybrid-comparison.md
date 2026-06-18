# Vector-First Retrieval With Optional Hybrid Comparison

The first product retrieval path will use `chromem-go` vector search: embed the user query, retrieve top-k chunks, pass selected context to the local LLM, and return an answer with citations. Reranking is optional and belongs to the evaluation or optimization path rather than the required v1 workflow. Keyword or hybrid retrieval may be added later as an experiment or fallback, but it is not required for the initial product workflow.

## Consequences

This keeps v1 achievable while preserving clear evaluation topics: vector-only retrieval can be compared against keyword or hybrid retrieval, and no-rerank retrieval can be compared against lightweight reranking using accuracy, citation correctness, and response-time metrics.

Multi-round chat should store sessions and messages in SQLite. Each new factual user message should build a retrieval query from the current question plus a short conversation summary or recent turns, retrieve fresh chunks, and cite the evidence used in that round. The LLM should not answer factual follow-ups only from chat history unless the answer is purely conversational.
