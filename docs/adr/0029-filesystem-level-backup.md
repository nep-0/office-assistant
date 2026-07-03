# Filesystem-Level Backup

The first version documents backup and restore at the backend mounted-volume level rather than providing a complex UI export feature. Stopping the stack and backing up the backend volume captures SQLite state, uploaded files, extracted Markdown, and vector index files owned by the backend.

