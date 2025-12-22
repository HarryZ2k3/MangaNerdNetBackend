package progress

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

func (r *Repo) Add(ctx context.Context, entry models.ProgressHistory) error {
	if entry.At.IsZero() {
		entry.At = time.Now().UTC()
	}

	var volume any
	if entry.Volume != nil {
		volume = *entry.Volume
	}

	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO user_progress_history (user_id, manga_id, chapter, volume, at)
		VALUES (?, ?, ?, ?, ?)
	`, entry.UserID, entry.MangaID, entry.Chapter, volume, entry.At)
	if err != nil {
		return fmt.Errorf("insert progress history: %w", err)
	}
	return nil
}

func (r *Repo) List(ctx context.Context, userID, mangaID string, limit, offset int) ([]models.ProgressHistory, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM user_progress_history
		WHERE user_id = ? AND manga_id = ?
	`, userID, mangaID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count progress history: %w", err)
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT user_id, manga_id, chapter, volume, at
		FROM user_progress_history
		WHERE user_id = ? AND manga_id = ?
		ORDER BY at DESC
		LIMIT ? OFFSET ?
	`, userID, mangaID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list progress history: %w", err)
	}
	defer rows.Close()

	out := make([]models.ProgressHistory, 0, limit)
	for rows.Next() {
		var entry models.ProgressHistory
		var volume sql.NullInt64
		var at time.Time

		if err := rows.Scan(&entry.UserID, &entry.MangaID, &entry.Chapter, &volume, &at); err != nil {
			return nil, 0, fmt.Errorf("scan progress history: %w", err)
		}
		if volume.Valid {
			v := int(volume.Int64)
			entry.Volume = &v
		}
		entry.At = at
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows progress history: %w", err)
	}

	return out, total, nil
}
