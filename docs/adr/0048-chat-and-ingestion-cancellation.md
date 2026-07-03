# Chat And Ingestion Cancellation

Chat streaming includes a user-facing stop control that cancels model generation when the provider supports cancellation. Ingestion cancellation is best-effort, allowing cancellation before irreversible or index-write stages where practical and otherwise marking cancellation pending before cleanup.

