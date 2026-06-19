package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `select count(*) from users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

func (s *Store) CreateUser(ctx context.Context, user User) (User, error) {
	if strings.TrimSpace(user.Username) == "" {
		return User{}, errors.New("username is required")
	}
	if strings.TrimSpace(user.PasswordHash) == "" {
		return User{}, errors.New("password_hash is required")
	}
	if user.Role != "admin" && user.Role != "user" {
		return User{}, errors.New("role must be admin or user")
	}
	if user.CreatedAt == "" {
		user.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	_, err := s.db.ExecContext(ctx, `insert into users (id, username, password_hash, role, created_at) values (?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.PasswordHash, user.Role, user.CreatedAt)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	return s.getUser(ctx, `select id, username, password_hash, role, created_at from users where username = ?`, username)
}

func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	return s.getUser(ctx, `select id, username, password_hash, role, created_at from users where id = ?`, id)
}

func (s *Store) getUser(ctx context.Context, query string, args ...any) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, sql.ErrNoRows
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `select id, username, password_hash, role, created_at from users order by created_at asc`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}
