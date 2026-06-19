package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret []byte
	ttl    time.Duration
}

func NewManager(secret string, ttl time.Duration) *Manager {
	if secret == "" {
		secret = "office-assistant-dev-secret"
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Manager{secret: []byte(secret), ttl: ttl}
}

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return errors.New("invalid username or password")
	}
	return nil
}

func (m *Manager) Issue(userID, username, role string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signed, nil
}

func (m *Manager) Verify(tokenValue string) (Claims, error) {
	token, err := jwt.ParseWithClaims(tokenValue, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("parse jwt: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return Claims{}, errors.New("invalid jwt")
	}
	return *claims, nil
}

type contextKey struct{}

func WithClaims(ctx context.Context, claims Claims) context.Context {
	return context.WithValue(ctx, contextKey{}, claims)
}

func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(contextKey{}).(Claims)
	return claims, ok
}
