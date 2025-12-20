package manga

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"mangahub/pkg/models"
)

type Repo struct {
	DB *sql.DB
}

type ListQuery struct {
	Q      string   // keyword search in title/author
	Genres []string // any-match
	Status string
	Limit  int
	Offset int
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{DB: db}
}

func (r *Repo) GetByID(ctx context.Context, id string) (*models.MangaDB, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, title, author, genres, status, total_chapters, description, cover_url
		FROM manga
		WHERE id = ?
	`, id)

	var (
		m           models.MangaDB
		author      sql.NullString
		genresJSON  string
		status      sql.NullString
		chapters    sql.NullInt64
		description sql.NullString
		coverURL    sql.NullString
	)

	if err := row.Scan(
		&m.ID, &m.Title, &author, &genresJSON, &status, &chapters, &description, &coverURL,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan getByID: %w", err)
	}

	m.Author = author.String
	m.Status = status.String
	if chapters.Valid {
		m.TotalChapters = int(chapters.Int64)
	}
	m.Description = description.String
	m.CoverURL = coverURL.String

	_ = json.Unmarshal([]byte(genresJSON), &m.Genres)
	return &m, nil
}

func (r *Repo) Count(ctx context.Context, q ListQuery) (int, error) {
	sqlStr, args := buildListSQL(q, true)
	row := r.DB.QueryRowContext(ctx, sqlStr, args...)
	var total int
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("count scan: %w", err)
	}
	return total, nil
}

func (r *Repo) List(ctx context.Context, q ListQuery) ([]models.MangaDB, error) {
	sqlStr, args := buildListSQL(q, false)

	rows, err := r.DB.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("list query: %w", err)
	}
	defer rows.Close()

	out := make([]models.MangaDB, 0, q.Limit)
	for rows.Next() {
		var (
			m           models.MangaDB
			author      sql.NullString
			genresJSON  string
			status      sql.NullString
			chapters    sql.NullInt64
			description sql.NullString
			coverURL    sql.NullString
		)

		if err := rows.Scan(
			&m.ID, &m.Title, &author, &genresJSON, &status, &chapters, &description, &coverURL,
		); err != nil {
			return nil, fmt.Errorf("list scan: %w", err)
		}

		m.Author = author.String
		m.Status = status.String
		if chapters.Valid {
			m.TotalChapters = int(chapters.Int64)
		}
		m.Description = description.String
		m.CoverURL = coverURL.String

		_ = json.Unmarshal([]byte(genresJSON), &m.Genres)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return out, nil
}

// buildListSQL builds either COUNT(*) or SELECT list.
// genres filter is "any-match" by doing LIKE searches inside stored JSON text.
func buildListSQL(q ListQuery, countOnly bool) (string, []any) {
	baseSelect := `
		SELECT id, title, author, genres, status, total_chapters, description, cover_url
		FROM manga
	`
	if countOnly {
		baseSelect = `SELECT COUNT(*) FROM manga`
	}

	var where []string
	var args []any

	if strings.TrimSpace(q.Q) != "" {
		where = append(where, "(LOWER(title) LIKE ? OR LOWER(author) LIKE ?)")
		kw := "%" + strings.ToLower(strings.TrimSpace(q.Q)) + "%"
		args = append(args, kw, kw)
	}

	if strings.TrimSpace(q.Status) != "" {
		where = append(where, "LOWER(status) = ?")
		args = append(args, strings.ToLower(strings.TrimSpace(q.Status)))
	}

	// any-match genre filter against JSON string
	if len(q.Genres) > 0 {
		var genreOr []string
		for _, g := range q.Genres {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}
			genreOr = append(genreOr, "LOWER(genres) LIKE ?")
			args = append(args, `%`+strings.ToLower(g)+`%`)
		}
		if len(genreOr) > 0 {
			where = append(where, "("+strings.Join(genreOr, " OR ")+")")
		}
	}

	sqlStr := baseSelect
	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}

	if !countOnly {
		sqlStr += " ORDER BY title ASC"
		sqlStr += " LIMIT ? OFFSET ?"
		limit := q.Limit
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		offset := q.Offset
		if offset < 0 {
			offset = 0
		}
		args = append(args, limit, offset)
	}

	return sqlStr, args
}
