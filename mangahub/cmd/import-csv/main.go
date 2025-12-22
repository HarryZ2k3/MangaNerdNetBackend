package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"mangahub/pkg/database"
)

func main() {
	var (
		mangaIn    = flag.String("manga", "data/manga.csv", "input CSV path for manga")
		progressIn = flag.String("progress", "data/user_progress.csv", "input CSV path for user progress")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db := database.MustOpen(database.DefaultConfig())
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("db migrate failed: %v", err)
	}

	if err := importManga(ctx, db, *mangaIn); err != nil {
		log.Fatalf("import manga failed: %v", err)
	}
	if err := importUserProgress(ctx, db, *progressIn); err != nil {
		log.Fatalf("import user progress failed: %v", err)
	}

	log.Printf("✅ imported manga from %s and user progress from %s", *mangaIn, *progressIn)
}

func importManga(ctx context.Context, db *sql.DB, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1

	header, err := readHeader(r)
	if err != nil {
		return err
	}

	stmt, err := db.PrepareContext(ctx, `
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
		return err
	}
	defer stmt.Close()

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(row) == 0 {
			continue
		}

		id := valueAt(header, row, "id")
		title := valueAt(header, row, "title")
		if id == "" || title == "" {
			continue
		}

		totalChapters, err := parseNullInt(valueAt(header, row, "total_chapters"))
		if err != nil {
			return fmt.Errorf("parse total_chapters for %s: %w", id, err)
		}

		if _, err := stmt.ExecContext(
			ctx,
			id,
			title,
			nullString(valueAt(header, row, "author")),
			nullString(valueAt(header, row, "genres")),
			nullString(valueAt(header, row, "status")),
			totalChapters,
			nullString(valueAt(header, row, "description")),
			nullString(valueAt(header, row, "cover_url")),
		); err != nil {
			return err
		}
	}

	return nil
}

func importUserProgress(ctx context.Context, db *sql.DB, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1

	header, err := readHeader(r)
	if err != nil {
		return err
	}

	stmt, err := db.PrepareContext(ctx, `
		INSERT INTO user_progress (user_id, manga_id, current_chapter, status, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, manga_id) DO UPDATE SET
			current_chapter = excluded.current_chapter,
			status = excluded.status,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(row) == 0 {
			continue
		}

		userID := valueAt(header, row, "user_id")
		mangaID := valueAt(header, row, "manga_id")
		if userID == "" || mangaID == "" {
			continue
		}

		currentChapter, err := parseNullInt(valueAt(header, row, "current_chapter"))
		if err != nil {
			return fmt.Errorf("parse current_chapter for %s/%s: %w", userID, mangaID, err)
		}

		updatedAt, err := parseTime(valueAt(header, row, "updated_at"))
		if err != nil {
			return fmt.Errorf("parse updated_at for %s/%s: %w", userID, mangaID, err)
		}

		if _, err := stmt.ExecContext(
			ctx,
			userID,
			mangaID,
			currentChapter,
			nullString(valueAt(header, row, "status")),
			updatedAt,
		); err != nil {
			return err
		}
	}

	return nil
}

func readHeader(r *csv.Reader) (map[string]int, error) {
	row, err := r.Read()
	if err != nil {
		return nil, err
	}
	header := make(map[string]int, len(row))
	for idx, name := range row {
		header[strings.TrimSpace(strings.ToLower(name))] = idx
	}
	return header, nil
}

func valueAt(header map[string]int, row []string, key string) string {
	idx, ok := header[key]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func parseNullInt(raw string) (sql.NullInt64, error) {
	if raw == "" {
		return sql.NullInt64{}, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return sql.NullInt64{}, err
	}
	return sql.NullInt64{Int64: n, Valid: true}, nil
}

func parseTime(raw string) (sql.NullTime, error) {
	if raw == "" {
		return sql.NullTime{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return sql.NullTime{}, err
	}
	return sql.NullTime{Time: t, Valid: true}, nil
}

func nullString(raw string) sql.NullString {
	if raw == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: raw, Valid: true}
}
