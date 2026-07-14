# Lightweight Go HTTP Stack

The backend uses Go's standard `net/http` with a lightweight router and middleware stack. The project avoids heavyweight web frameworks because the main complexity is document state, background jobs, retrieval, and agent orchestration rather than web framework features.
