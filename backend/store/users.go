package store

import (
	"context"
	"database/sql"
	"time"
)

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = ?`, roleAdmin).Scan(&count)
	return count, err
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash, role string) (User, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO users (username, password_hash, role)
VALUES (?, ?, ?)
`, username, passwordHash, role)
	if err != nil {
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return s.FindUserByID(ctx, id)
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, username, password_hash, role, created_at
FROM users
ORDER BY id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Store) FindUserByUsername(ctx context.Context, username string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, created_at
FROM users
WHERE username = ?
`, username))
}

func (s *Store) FindUserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, created_at
FROM users
WHERE id = ?
`, id))
}

func (s *Store) UpdateUser(ctx context.Context, user User) (User, error) {
	res, err := s.db.ExecContext(ctx, `
UPDATE users
SET username = ?, password_hash = ?, role = ?
WHERE id = ?
`, user.Username, user.PasswordHash, user.Role, user.ID)
	if err != nil {
		return User{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return User{}, err
	}
	if affected == 0 {
		return User{}, sql.ErrNoRows
	}
	return s.FindUserByID(ctx, user.ID)
}

func (s *Store) CountKnowledgeBasesOwnedByUser(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM knowledge_bases
WHERE owner_user_id = ? AND deleted_at IS NULL
`, userID).Scan(&count)
	return count, err
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *Store) CreateSession(ctx context.Context, id string, userID int64, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, expires_at)
VALUES (?, ?, ?)
`, id, userID, expiresAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) FindUserBySession(ctx context.Context, sessionID string, now time.Time) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
SELECT users.id, users.username, users.password_hash, users.role, users.created_at
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.id = ? AND sessions.expires_at > ?
`, sessionID, now.UTC().Format(time.RFC3339)))
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID)
	return err
}
