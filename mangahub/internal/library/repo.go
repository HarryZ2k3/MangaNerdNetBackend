package library

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"mangahub/pkg/models"
)

type Repo struct {
	DB *sql.DB
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{DB: db}
}

// Upsert inserts or updates a user's library item
func (r *Repo) Upsert(ctx context.Context, item models.LibraryItem) error {
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO user_progress (user_id, manga_id, current_chapter, status, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, manga_id) DO UPDATE SET
			current_chapter = excluded.current_chapter,
			status = excluded.status,
			updated_at = CURRENT_TIMESTAMP
	`, item.UserID, item.MangaID, item.CurrentChapter, item.Status)
	if err != nil {
		return fmt.Errorf("upsert library item: %w", err)
	}
	return nil
}

func (r *Repo) Delete(ctx context.Context, userID, mangaID string) (bool, error) {
	res, err := r.DB.ExecContext(ctx, `
		DELETE FROM user_progress
		WHERE user_id = ? AND manga_id = ?
	`, userID, mangaID)
	if err != nil {
		return false, fmt.Errorf("delete library item: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *Repo) List(ctx context.Context, userID string, status string, limit, offset int) ([]models.LibraryItem, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	// count
	var total int
	var countErr error
	if status == "" {
		countErr = r.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM user_progress WHERE user_id = ?
		`, userID).Scan(&total)
	} else {
		countErr = r.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM user_progress WHERE user_id = ? AND status = ?
		`, userID, status).Scan(&total)
	}
	if countErr != nil {
		return nil, 0, fmt.Errorf("count library: %w", countErr)
	}

	// list
	var rows *sql.Rows
	var err error

	if status == "" {
		rows, err = r.DB.QueryContext(ctx, `
			SELECT user_id, manga_id, current_chapter, status, updated_at
			FROM user_progress
			WHERE user_id = ?
			ORDER BY updated_at DESC
			LIMIT ? OFFSET ?
		`, userID, limit, offset)
	} else {
		rows, err = r.DB.QueryContext(ctx, `
			SELECT user_id, manga_id, current_chapter, status, updated_at
			FROM user_progress
			WHERE user_id = ? AND status = ?
			ORDER BY updated_at DESC
			LIMIT ? OFFSET ?
		`, userID, status, limit, offset)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("list library: %w", err)
	}
	defer rows.Close()

	out := make([]models.LibraryItem, 0, limit)
	for rows.Next() {
		var it models.LibraryItem
		var updated time.Time

		if err := rows.Scan(&it.UserID, &it.MangaID, &it.CurrentChapter, &it.Status, &updated); err != nil {
			return nil, 0, fmt.Errorf("scan library row: %w", err)
		}
		it.UpdatedAt = updated
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows err: %w", err)
	}

	return out, total, nil
}

func (r *Repo) Get(ctx context.Context, userID, mangaID string) (*models.LibraryItem, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT user_id, manga_id, current_chapter, status, updated_at
		FROM user_progress
		WHERE user_id = ? AND manga_id = ?
	`, userID, mangaID)

	var it models.LibraryItem
	var updated time.Time
	if err := row.Scan(&it.UserID, &it.MangaID, &it.CurrentChapter, &it.Status, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get library item: %w", err)
	}
	it.UpdatedAt = updated
	return &it, nil
}
