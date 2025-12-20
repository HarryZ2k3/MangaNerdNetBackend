package auth

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type User struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type Repo struct {
	DB *sql.DB
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{DB: db}
}

func (r *Repo) CreateUser(ctx context.Context, u User) error {
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO users (id, username, email, password_hash)
		VALUES (?, ?, ?, ?)
	`, u.ID, u.Username, u.Email, u.PasswordHash)

	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *Repo) GetByEmail(ctx context.Context, email string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, created_at
		FROM users
		WHERE LOWER(email) = ?
	`, email)

	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get by email: %w", err)
	}
	return &u, nil
}

func (r *Repo) GetByUsername(ctx context.Context, username string) (*User, error) {
	username = strings.TrimSpace(username)
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, created_at
		FROM users
		WHERE username = ?
	`, username)

	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get by username: %w", err)
	}
	return &u, nil
}
