# Model Provider Configuration

Model provider configuration uses environment variables for first-boot and deployment defaults, then stores active admin-editable settings in backend SQLite. Secrets remain backend-side and are masked in the UI, allowing the admin to switch between cloud and local OpenAI-compatible providers without rebuilding containers or exposing keys in frontend assets.

