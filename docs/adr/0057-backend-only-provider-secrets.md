# Backend-Only Provider Secrets

Cloud provider API keys are never exposed to frontend assets, never logged, and are masked in admin settings. For the first version, environment variables are preferred for secrets while SQLite stores non-secret provider settings, unless simple backend-side encrypted or obfuscated secret storage is feasible without distracting from the final-year project scope.

