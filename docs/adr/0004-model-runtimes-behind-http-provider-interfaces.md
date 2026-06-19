# Model Runtimes Behind HTTP Provider Interfaces

The Go backend will call model runtimes through Eino-backed HTTP/provider interfaces instead of embedding model execution directly. The first provider contract should use Eino's OpenAI-compatible chat and embedding components so cloud providers and local llama.cpp-compatible servers can share the same integration path. LLM generation and embedding are expected to run in separate local containers for the offline path, with llama.cpp containers as the initial candidate for both generation and embedding models.

## Consequences

The backend depends on stable provider contracts rather than a specific model binary. Eino is the Go integration layer for model components, while the application keeps its own narrow provider interfaces for chat streaming and embeddings. This supports fully offline deployment while allowing the project to compare or replace model runtimes without rewriting the application workflow.
