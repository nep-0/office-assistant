package app

import (
	"context"
	"errors"
	"strings"

	"office-assistant/internal/auth"
	"office-assistant/internal/storage"
	"office-assistant/internal/utils"
)

type AccountStore interface {
	CountUsers(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, user storage.User) (storage.User, error)
	GetUserByUsername(ctx context.Context, username string) (storage.User, error)
	GetUserByID(ctx context.Context, id string) (storage.User, error)
	ListUsers(ctx context.Context) ([]storage.User, error)
}

type AccountService struct {
	store  AccountStore
	tokens *auth.Manager
}

type AuthResult struct {
	Token string
	User  storage.User
}

func NewAccountService(store AccountStore, tokens *auth.Manager) *AccountService {
	return &AccountService{store: store, tokens: tokens}
}

func (s *AccountService) BootstrapAdmin(ctx context.Context, username, password string) (AuthResult, error) {
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return AuthResult{}, err
	}
	if count > 0 {
		return AuthResult{}, ConflictError{Message: "bootstrap is only available before users exist"}
	}

	user, err := s.CreateUser(ctx, username, password, "admin")
	if err != nil {
		return AuthResult{}, err
	}
	token, err := s.tokens.Issue(user.ID, user.Username, user.Role)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{Token: token, User: user}, nil
}

func (s *AccountService) Login(ctx context.Context, username, password string) (AuthResult, error) {
	user, err := s.store.GetUserByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		return AuthResult{}, PermissionError{Status: "unauthorized", Message: "invalid username or password"}
	}
	if err := auth.CheckPassword(user.PasswordHash, password); err != nil {
		return AuthResult{}, PermissionError{Status: "unauthorized", Message: err.Error()}
	}
	token, err := s.tokens.Issue(user.ID, user.Username, user.Role)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{Token: token, User: user}, nil
}

func (s *AccountService) Me(ctx context.Context, userID string) (storage.User, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return storage.User{}, PermissionError{Status: "unauthorized", Message: "user no longer exists"}
	}
	return user, nil
}

func (s *AccountService) ListUsers(ctx context.Context) ([]storage.User, error) {
	return s.store.ListUsers(ctx)
}

func (s *AccountService) CreateUser(ctx context.Context, username, password, role string) (storage.User, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return storage.User{}, err
	}
	return s.store.CreateUser(ctx, storage.User{
		ID:           utils.NewID("usr"),
		Username:     strings.TrimSpace(username),
		PasswordHash: hash,
		Role:         strings.TrimSpace(role),
	})
}

type ConflictError struct {
	Message string
}

func (e ConflictError) Error() string {
	if e.Message == "" {
		return "conflict"
	}
	return e.Message
}

func IsConflict(err error) bool {
	var conflict ConflictError
	return errors.As(err, &conflict)
}
