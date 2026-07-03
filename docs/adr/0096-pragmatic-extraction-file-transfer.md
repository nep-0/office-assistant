# Pragmatic Extraction File Transfer

Backend-to-document extraction requests should stream file content where reasonable and avoid loading large files entirely into memory, while relying primarily on upload size limits and HTTP timeouts for protection. The first version does not require resumable or chunked extraction protocols.

