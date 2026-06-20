# Vector-First Retrieval With Optional Hybrid Comparison

The first product retrieval path will expose retrieval as a tool abstraction backed by `chromem-go` vector search. The chat workflow should not force retrieval before generation; retrieval runs only when the model requests the retrieval tool, then the returned chunks are used for grounding and citations. Reranking is optional and belongs to the evaluation or optimization path rather than the required v1 workflow. Keyword or hybrid retrieval may be added later as an experiment or fallback, but it is not required for the initial product workflow.

## Consequences

This keeps v1 achievable while preserving clear evaluation topics: vector-only retrieval can be compared against keyword or hybrid retrieval, and no-rerank retrieval can be compared against lightweight reranking using accuracy, citation correctness, and response-time metrics.

Multi-round chat should store sessions and messages in SQLite. For each new factual user message, the model may request retrieval using the current question plus a short conversation summary or recent turns. When it does request retrieval, the answer must cite the evidence used in that round. The LLM may answer purely conversational turns without calling retrieval.

Retrieval is modeled as a tool that the model can call. The current spike uses a deterministic static retrieval tool triggered by the mock provider's tool-call event, but the contract is shaped so it can later be backed by `chromem-go` and exposed through Eino tool-calling. This keeps retrieval as an explicit capability rather than hidden prompt assembly inside the HTTP layer.
