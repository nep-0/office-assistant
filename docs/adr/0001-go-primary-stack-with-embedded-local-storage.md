# Go Primary Stack With Embedded Local Storage

The project will use Go as the main implementation language, SQLite for application metadata, and `chromem-go` for local vector indexing. This deliberately shifts the system away from a Python-first AI pipeline so the final deployment can be a simpler local service with fewer runtime components, while accepting that some document parsing or OCR tools may still need to be called through separate libraries or helper processes.

## Considered Options

- Python-first backend with a separate vector database such as Chroma or Qdrant.
- Go-first backend with SQLite and embedded vector search through `chromem-go`.

## Consequences

Go becomes the default language for backend orchestration, API design, metadata storage, indexing workflow, and Docker packaging. Any Python usage should be treated as an integration dependency rather than the center of the application architecture.
