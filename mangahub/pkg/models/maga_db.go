package models

type MangaDB struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Author        string   `json:"author,omitempty"`
	Genres        []string `json:"genres"`
	Status        string   `json:"status,omitempty"`
	TotalChapters int      `json:"total_chapters,omitempty"`
	Description   string   `json:"description,omitempty"`
	CoverURL      string   `json:"cover_url,omitempty"`
}
