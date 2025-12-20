package sync

import "time"

type LibraryEvent struct {
	Type           string    `json:"type"` // "library.update" or "library.delete"
	UserID         string    `json:"user_id"`
	MangaID        string    `json:"manga_id"`
	CurrentChapter int       `json:"current_chapter,omitempty"`
	Status         string    `json:"status,omitempty"`
	At             time.Time `json:"at"`
}
