# Cloud Models as Admin-Enabled Optional Providers

Cloud embedding and cloud LLM calls may be implemented as optional providers that an admin can enable or disable. The system must remain fully usable through local offline providers, but cloud providers are not limited to the early prototype.

## Consequences

The backend should define model-provider interfaces early so local llama.cpp-compatible providers and cloud providers share the same document, indexing, retrieval, and chat workflows. Evaluation results must clearly state whether local or cloud providers were used; cloud-provider results should not be used as evidence for the final offline deployment claim.
