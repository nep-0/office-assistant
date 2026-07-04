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

type providerSetting struct {
	Purpose   string
	BaseURL   string
	Model     string
	APIKey    string
	UpdatedAt time.Time
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

CREATE TABLE IF NOT EXISTS provider_settings (
	purpose TEXT PRIMARY KEY CHECK (purpose IN ('chat', 'embedding')),
	base_url TEXT NOT NULL,
	model TEXT NOT NULL,
	api_key TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
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

func (s *store) ensureProviderDefaults(ctx context.Context, defaults map[string]providerSetting) error {
	for purpose, setting := range defaults {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO provider_settings (purpose, base_url, model, api_key)
VALUES (?, ?, ?, ?)
ON CONFLICT(purpose) DO NOTHING
`, purpose, setting.BaseURL, setting.Model, setting.APIKey)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *store) listProviderSettings(ctx context.Context) ([]providerSetting, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT purpose, base_url, model, api_key, updated_at
FROM provider_settings
ORDER BY purpose
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []providerSetting
	for rows.Next() {
		setting, err := scanProviderSetting(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	return settings, rows.Err()
}

func (s *store) findProviderSetting(ctx context.Context, purpose string) (providerSetting, error) {
	return scanProviderSetting(s.db.QueryRowContext(ctx, `
SELECT purpose, base_url, model, api_key, updated_at
FROM provider_settings
WHERE purpose = ?
`, purpose))
}

func (s *store) updateProviderSetting(ctx context.Context, setting providerSetting) (providerSetting, error) {
	_, err := s.db.ExecContext(ctx, `
UPDATE provider_settings
SET base_url = ?, model = ?, api_key = ?, updated_at = CURRENT_TIMESTAMP
WHERE purpose = ?
`, setting.BaseURL, setting.Model, setting.APIKey, setting.Purpose)
	if err != nil {
		return providerSetting{}, err
	}
	return s.findProviderSetting(ctx, setting.Purpose)
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
	parsed, err := parseSQLiteTime(createdAt)
	if err != nil {
		return user{}, err
	}
	u.CreatedAt = parsed
	return u, nil
}

func scanProviderSetting(row rowScanner) (providerSetting, error) {
	var setting providerSetting
	var updatedAt string
	if err := row.Scan(&setting.Purpose, &setting.BaseURL, &setting.Model, &setting.APIKey, &updatedAt); err != nil {
		return providerSetting{}, err
	}
	parsed, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return providerSetting{}, err
	}
	setting.UpdatedAt = parsed
	return setting, nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02 15:04:05", value)
}
