package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"mangahub/pkg/database"
)

func main() {
	var (
		mangaOut    = flag.String("manga", "data/manga.csv", "output CSV path for manga")
		progressOut = flag.String("progress", "data/user_progress.csv", "output CSV path for user progress")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db := database.MustOpen(database.DefaultConfig())
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("db migrate failed: %v", err)
	}

	if err := exportManga(ctx, db, *mangaOut); err != nil {
		log.Fatalf("export manga failed: %v", err)
	}
	if err := exportUserProgress(ctx, db, *progressOut); err != nil {
		log.Fatalf("export user progress failed: %v", err)
	}

	log.Printf("✅ exported manga to %s and user progress to %s", *mangaOut, *progressOut)
}

func exportManga(ctx context.Context, db *sql.DB, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"id", "title", "author", "genres", "status", "total_chapters", "description", "cover_url"}); err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, `
        SELECT id, title, author, genres, status, total_chapters, description, cover_url
        FROM manga
        ORDER BY title
    `)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id            string
			title         string
			author        sql.NullString
			genres        sql.NullString
			status        sql.NullString
			totalChapters sql.NullInt64
			description   sql.NullString
			coverURL      sql.NullString
		)

		if err := rows.Scan(&id, &title, &author, &genres, &status, &totalChapters, &description, &coverURL); err != nil {
			return err
		}

		total := ""
		if totalChapters.Valid {
			total = strconv.FormatInt(totalChapters.Int64, 10)
		}

		if err := w.Write([]string{
			id,
			title,
			author.String,
			genres.String,
			status.String,
			total,
			description.String,
			coverURL.String,
		}); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	w.Flush()
	return w.Error()
}

func exportUserProgress(ctx context.Context, db *sql.DB, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"user_id", "manga_id", "current_chapter", "status", "updated_at"}); err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, `
        SELECT user_id, manga_id, current_chapter, status, updated_at
        FROM user_progress
        ORDER BY updated_at DESC
    `)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			userID         string
			mangaID        string
			currentChapter sql.NullInt64
			status         sql.NullString
			updatedAt      sql.NullTime
		)

		if err := rows.Scan(&userID, &mangaID, &currentChapter, &status, &updatedAt); err != nil {
			return err
		}

		chapter := ""
		if currentChapter.Valid {
			chapter = strconv.FormatInt(currentChapter.Int64, 10)
		}

		updated := ""
		if updatedAt.Valid {
			updated = updatedAt.Time.Format(time.RFC3339)
		}

		if err := w.Write([]string{
			userID,
			mangaID,
			chapter,
			status.String,
			updated,
		}); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	w.Flush()
	return w.Error()
}
