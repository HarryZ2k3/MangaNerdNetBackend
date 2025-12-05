package scraper

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mangahub/pkg/models"
)

// SaveToDatabase upserts the given slice of MangaCanonical into the
// `manga` table using the schema defined in the project spec:
//
//	CREATE TABLE manga (
//	  id TEXT PRIMARY KEY,
//	  title TEXT,
//	  author TEXT,
//	  genres TEXT, -- JSON array as text
//	  status TEXT,
//	  total_chapters INTEGER,
//	  description TEXT
//	);
//
// This function assumes you may have added "cover_url" as an extra column.
func SaveToDatabase(ctx context.Context, db *sql.DB, mangas []models.MangaCanonical) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO manga (id, title, author, genres, status, total_chapters, description, cover_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  title = excluded.title,
		  author = excluded.author,
		  genres = excluded.genres,
		  status = excluded.status,
		  total_chapters = excluded.total_chapters,
		  description = excluded.description,
		  cover_url = excluded.cover_url
	`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, m := range mangas {
		genresJSON, err := json.Marshal(m.Genres)
		if err != nil {
			return fmt.Errorf("marshal genres for %s: %w", m.ID, err)
		}

		if _, err := stmt.ExecContext(
			ctx,
			m.ID,
			m.Title,
			m.Author,
			string(genresJSON),
			m.Status,
			m.TotalChapters,
			m.Description,
			m.CoverURL,
		); err != nil {
			return fmt.Errorf("exec upsert for %s: %w", m.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
