package reviews

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

func (r *Repo) Create(ctx context.Context, userID, mangaID string, rating int, text string) (*models.Review, error) {
	res, err := r.DB.ExecContext(ctx, `
		INSERT INTO reviews (user_id, manga_id, rating, text)
		VALUES (?, ?, ?, ?)
	`, userID, mangaID, rating, text)
	if err != nil {
		return nil, fmt.Errorf("insert review: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	return r.GetByID(ctx, id)
}

func (r *Repo) GetByID(ctx context.Context, id int64) (*models.Review, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, user_id, manga_id, rating, text, timestamp
		FROM reviews
		WHERE id = ?
	`, id)

	var review models.Review
	var text sql.NullString
	var ts time.Time
	if err := row.Scan(&review.ID, &review.UserID, &review.MangaID, &review.Rating, &text, &ts); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan review: %w", err)
	}

	review.Text = text.String
	review.Timestamp = ts
	return &review, nil
}

func (r *Repo) ListByManga(ctx context.Context, mangaID string, limit, offset int) ([]models.Review, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, user_id, manga_id, rating, text, timestamp
		FROM reviews
		WHERE manga_id = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, mangaID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	defer rows.Close()

	out := make([]models.Review, 0, limit)
	for rows.Next() {
		var review models.Review
		var text sql.NullString
		var ts time.Time

		if err := rows.Scan(&review.ID, &review.UserID, &review.MangaID, &review.Rating, &text, &ts); err != nil {
			return nil, fmt.Errorf("scan review row: %w", err)
		}

		review.Text = text.String
		review.Timestamp = ts
		out = append(out, review)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return out, nil
}

func (r *Repo) Delete(ctx context.Context, id int64, userID string) (bool, error) {
	res, err := r.DB.ExecContext(ctx, `
		DELETE FROM reviews
		WHERE id = ? AND user_id = ?
	`, id, userID)
	if err != nil {
		return false, fmt.Errorf("delete review: %w", err)
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}
