package models

import "time"

type Review struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`
	MangaID   string    `json:"manga_id"`
	Rating    int       `json:"rating"`
	Text      string    `json:"text,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
