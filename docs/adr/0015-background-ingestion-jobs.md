# Background Ingestion Jobs

Document upload creates a stored file and document record quickly, then backend workers perform extraction, chunking, embedding, and indexing as background ingestion jobs. The UI reflects job states such as pending, processing, ready, and failed, including failure details when ingestion cannot complete.

