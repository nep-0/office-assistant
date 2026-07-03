# ID-Based File Storage

Uploaded originals and generated artifacts are stored on disk using internal IDs and document version paths rather than raw user filenames. Original filename, display name, MIME or detected type, size, and hash are stored in SQLite for UI display, rename behavior, and possible deduplication.

