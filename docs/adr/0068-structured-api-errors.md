# Structured API Errors

Backend APIs return structured JSON errors with a stable `code`, an English developer-readable `message`, and optional `details` for field errors, job IDs, dependency names, or retry hints. Frontend localization and workflow handling depend on the stable error code rather than parsing message text.

