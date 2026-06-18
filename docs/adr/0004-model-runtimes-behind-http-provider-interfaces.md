# Model Runtimes Behind HTTP Provider Interfaces

The Go backend will call model runtimes through HTTP/provider interfaces instead of embedding model execution directly. The first provider contract should use OpenAI-compatible API shapes so cloud providers and local llama.cpp-compatible servers can share the same integration path. LLM generation and embedding are expected to run in separate local containers for the offline path, with llama.cpp containers as the initial candidate for both generation and embedding models.

## Consequences

The backend depends on stable request/response contracts rather than a specific model binary. This supports fully offline deployment while allowing the project to compare or replace model runtimes without rewriting the application workflow.
