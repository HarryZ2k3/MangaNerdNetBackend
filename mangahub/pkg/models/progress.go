package models

import "time"

type ProgressHistory struct {
	UserID  string    `json:"user_id"`
	MangaID string    `json:"manga_id"`
	Chapter int       `json:"chapter"`
	Volume  *int      `json:"volume,omitempty"`
	At      time.Time `json:"at"`
}
