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
	TokenVersion int
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
		SELECT id, username, email, password_hash, token_version, created_at
		FROM users
		WHERE LOWER(email) = ?
	`, email)

	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.TokenVersion, &u.CreatedAt); err != nil {
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
		SELECT id, username, email, password_hash, token_version, created_at
		FROM users
		WHERE username = ?
	`, username)

	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.TokenVersion, &u.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get by username: %w", err)
	}
	return &u, nil
}

func (r *Repo) GetByID(ctx context.Context, id string) (*User, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, token_version, created_at
		FROM users
		WHERE id = ?
	`, id)

	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.TokenVersion, &u.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get by id: %w", err)
	}
	return &u, nil
}

func (r *Repo) GetTokenVersion(ctx context.Context, id string) (int, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT token_version
		FROM users
		WHERE id = ?
	`, id)

	var version int
	if err := row.Scan(&version); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("get token version: %w", err)
	}
	return version, nil
}

func (r *Repo) UpdatePasswordAndBumpTokenVersion(ctx context.Context, id string, passwordHash string) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update password: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET password_hash = ?, token_version = token_version + 1
		WHERE id = ?
	`, passwordHash, id)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update password rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("update password: user not found")
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit update password: %w", err)
	}
	return nil
}

func (r *Repo) BumpTokenVersion(ctx context.Context, id string) error {
	res, err := r.DB.ExecContext(ctx, `
		UPDATE users
		SET token_version = token_version + 1
		WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("bump token version: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("bump token version rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("bump token version: user not found")
	}
	return nil
}
