# Agentic Retrieval Tool

The backend uses an agentic question-answering flow where the model can call a constrained retrieval tool instead of always receiving a fixed retrieval result assembled from the full chat history. This supports multi-turn chat while letting the model formulate focused search queries, with the backend still owning orchestration, tool execution, knowledge-base scope, and citation handling.

