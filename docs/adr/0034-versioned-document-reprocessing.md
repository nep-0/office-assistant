# Versioned Document Reprocessing

Document reprocessing builds a new internal version while the previous ready version remains searchable. Retrieval switches to the new chunks and vector entries only after reprocessing succeeds; if reprocessing fails, the old version remains active and the failure is shown to the user.

