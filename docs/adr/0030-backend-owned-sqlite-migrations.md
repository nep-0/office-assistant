# Backend-Owned SQLite Migrations

SQLite schema migrations are owned by the backend codebase and applied automatically on startup. Startup fails with a clear error if migration cannot complete, avoiding manual SQL steps for one-click local deployment while leaving downgrade support out of scope for the final-year project.

