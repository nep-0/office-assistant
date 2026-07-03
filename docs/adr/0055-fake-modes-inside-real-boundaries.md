# Fake Modes Inside Real Boundaries

Stub behavior lives inside real service boundaries where practical rather than in a parallel test-only architecture. The document service can expose a fake extractor mode, and the backend can use provider configuration to point at deterministic OpenAI-compatible fake chat or embedding providers when integration testing requires them.

