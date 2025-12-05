package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mangahub/pkg/models"
)

// SourceA is an example of a JSON API source.
// Adjust BaseURL and the raw struct to match the real API you use.
type SourceA struct {
	BaseURL string
	Client  *http.Client
}

// NewSourceA creates a new SourceA with the given base URL.
func NewSourceA(baseURL string) *SourceA {
	return &SourceA{
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *SourceA) Name() string {
	return "source_a"
}

// FetchAll fetches all manga from SourceA's API and maps them into MangaCanonical.
//
// This assumes an endpoint like:
//
//	GET {BaseURL}/manga
//
// that returns JSON array of objects.
func (s *SourceA) FetchAll(ctx context.Context) ([]models.MangaCanonical, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/manga", nil)
	if err != nil {
		return nil, fmt.Errorf("source_a: build request: %w", err)
	}

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("source_a: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("source_a: status %d: %s", resp.StatusCode, string(body))
	}

	// TODO: adapt this struct to the real API you use.
	var raw []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Author      string   `json:"author"`
		Genres      []string `json:"genres"`
		Status      string   `json:"status"`
		Chapters    int      `json:"chapters"`
		Description string   `json:"description"`
		Cover       string   `json:"cover"`
		Year        int      `json:"year"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("source_a: decode json: %w", err)
	}

	result := make([]models.MangaCanonical, 0, len(raw))
	for _, r := range raw {
		if r.ID == "" || r.Title == "" {
			// skip obviously broken records
			continue
		}
		m := models.MangaCanonical{
			ID:            r.ID, // you may want to slugify this
			Title:         r.Title,
			Author:        r.Author,
			Genres:        r.Genres,
			Status:        normalizeStatus(r.Status),
			TotalChapters: r.Chapters,
			Description:   r.Description,
			CoverURL:      r.Cover,
			Year:          r.Year,
			SourceIDs:     map[string]string{"source_a": r.ID},
		}
		result = append(result, m)
	}
	return result, nil
}

// normalizeStatus maps arbitrary status strings from the source into
// our internal values ("ongoing", "completed", "hiatus", etc.).
func normalizeStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ongoing", "publishing", "current":
		return "ongoing"
	case "finished", "completed", "complete":
		return "completed"
	case "hiatus":
		return "hiatus"
	default:
		return s
	}
}
