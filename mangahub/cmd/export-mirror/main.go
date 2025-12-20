package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mangahub/pkg/database"
)

type MirrorTitle struct {
	Slug          string   `json:"slug"`
	Name          string   `json:"name"`
	AltNames      []string `json:"alt_names"`
	Creator       string   `json:"creator"`
	Tags          []string `json:"tags"`
	State         string   `json:"state"`
	TotalChapters string   `json:"total_chapters"`
	Summary       string   `json:"summary"`
	ImageURL      string   `json:"image_url"`
	Year          string   `json:"year"`
}

func main() {
	var (
		outPath = flag.String("out", "data/mirror.json", "output JSON path")
		limit   = flag.Int("limit", 200, "how many titles to export")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.MustOpen(database.DefaultConfig())
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("db migrate failed: %v", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, title, author, genres, status, total_chapters, description, cover_url
		FROM manga
		ORDER BY title
		LIMIT ?
	`, *limit)
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	var out []MirrorTitle
	for rows.Next() {
		var (
			id            string
			title         string
			author        sql.NullString
			genresJSON    string
			status        sql.NullString
			totalChapters sql.NullInt64
			desc          sql.NullString
			coverURL      sql.NullString
		)

		if err := rows.Scan(&id, &title, &author, &genresJSON, &status, &totalChapters, &desc, &coverURL); err != nil {
			log.Fatalf("scan failed: %v", err)
		}

		var genres []string
		_ = json.Unmarshal([]byte(genresJSON), &genres)

		out = append(out, MirrorTitle{
			Slug:          toSlug(id, title),
			Name:          title,
			AltNames:      []string{},
			Creator:       author.String,
			Tags:          genres,
			State:         status.String,
			TotalChapters: itoaOrEmpty(totalChapters),
			Summary:       desc.String,
			ImageURL:      coverURL.String,
			Year:          "",
		})
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows error: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		log.Fatalf("mkdir failed: %v", err)
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		log.Fatalf("marshal failed: %v", err)
	}

	if err := os.WriteFile(*outPath, b, 0o644); err != nil {
		log.Fatalf("write failed: %v", err)
	}

	log.Printf("✅ exported %d titles to %s", len(out), *outPath)
}

func toSlug(id, title string) string {
	// Prefer using DB id if it already looks “slug-like”.
	// If DB id is a UUID (MangaDex), use a title-based slug for mirror dataset.
	if looksLikeUUID(id) {
		return slugify(title)
	}
	return slugify(id)
}

func looksLikeUUID(s string) bool {
	// quick heuristic; good enough for this tool
	return len(s) >= 32 && strings.Count(s, "-") >= 3
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else {
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "untitled"
	}
	return out
}

func itoaOrEmpty(n sql.NullInt64) string {
	if !n.Valid || n.Int64 <= 0 {
		return ""
	}
	return strconv.FormatInt(n.Int64, 10)
}
