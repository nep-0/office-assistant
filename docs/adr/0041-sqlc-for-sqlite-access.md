# sqlc For SQLite Access

The backend uses explicit SQL with `sqlc`-generated Go code for SQLite access rather than a heavy ORM. This keeps schema, migrations, indexes, and transactions visible while still providing typed query methods for application code.

