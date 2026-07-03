# Runnable Stubs For Vertical Skeleton

The first vertical skeleton uses runnable stub services or backend-configurable fake providers so Compose and the frontend exercise the same HTTP paths as the real system. Fake document extraction returns predictable Markdown, and fake chat or embedding providers expose deterministic OpenAI-compatible behavior for integration testing without real models.

