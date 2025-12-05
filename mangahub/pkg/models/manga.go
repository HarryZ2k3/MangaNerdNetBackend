package models

// MangaCanonical is the normalized, internal form of a manga entry
// used by the scraper and database layer.
//
// All external sources are mapped into this structure first,
// then we write to the DB from this representation.
type MangaCanonical struct {
	ID            string            `json:"id"`                   // our canonical ID (slug)
	Title         string            `json:"title"`                // main title
	AltTitles     []string          `json:"alt_titles,omitempty"` // alternative titles from other sources
	Author        string            `json:"author"`               // primary author / mangaka
	Genres        []string          `json:"genres"`               // normalized genre list
	Status        string            `json:"status"`               // "ongoing", "completed", etc.
	TotalChapters int               `json:"total_chapters"`       // best guess across sources
	Description   string            `json:"description"`          // combined/longest description
	CoverURL      string            `json:"cover_url,omitempty"`  // cover image URL (if any)
	Year          int               `json:"year,omitempty"`       // publication start year (optional)
	SourceIDs     map[string]string `json:"source_ids,omitempty"` // e.g. {"source_a": "...", "source_b": "..."}
}
