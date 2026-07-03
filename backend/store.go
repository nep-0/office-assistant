package main

import (
	"context"
	"database/sql"
	"time"
)

type store struct {
	db *sql.DB
}

type user struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *store) Close() error {
	return s.db.Close()
}

func (s *store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('admin', 'member')),
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at);
`)
	return err
}

func (s *store) countUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (s *store) createUser(ctx context.Context, username, passwordHash, role string) (user, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO users (username, password_hash, role)
VALUES (?, ?, ?)
`, username, passwordHash, role)
	if err != nil {
		return user{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return user{}, err
	}
	return s.findUserByID(ctx, id)
}

func (s *store) findUserByUsername(ctx context.Context, username string) (user, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, created_at
FROM users
WHERE username = ?
`, username))
}

func (s *store) findUserByID(ctx context.Context, id int64) (user, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, created_at
FROM users
WHERE id = ?
`, id))
}

func (s *store) createSession(ctx context.Context, id string, userID int64, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, expires_at)
VALUES (?, ?, ?)
`, id, userID, expiresAt.UTC().Format(time.RFC3339))
	return err
}

func (s *store) findUserBySession(ctx context.Context, sessionID string, now time.Time) (user, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
SELECT users.id, users.username, users.password_hash, users.role, users.created_at
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.id = ? AND sessions.expires_at > ?
`, sessionID, now.UTC().Format(time.RFC3339)))
}

func (s *store) deleteSession(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (user, error) {
	var u user
	var createdAt string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt); err != nil {
		return user{}, err
	}
	parsed, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		parsed, err = time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return user{}, err
		}
	}
	u.CreatedAt = parsed
	return u, nil
}
