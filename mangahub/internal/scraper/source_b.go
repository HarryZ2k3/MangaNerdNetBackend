package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mangahub/pkg/models"
)

// SourceB is a second source with a different JSON shape.
// For example, your own hosted JSON or another public API.
type SourceB struct {
	BaseURL string
	Client  *http.Client
}

// NewSourceB creates a new SourceB.
func NewSourceB(baseURL string) *SourceB {
	return &SourceB{
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *SourceB) Name() string {
	return "source_b"
}

// FetchAll fetches and maps SourceB's data into MangaCanonical.
//
// Example assumed response format:
//
//	GET {BaseURL}/titles
//	[
//	  {
//	    "slug": "one-piece",
//	    "name": "One Piece",
//	    "alt_names": ["ワンピース"],
//	    "creator": "Oda Eiichiro",
//	    "tags": ["Action", "Adventure"],
//	    "state": "finished",
//	    "total_chapters": "1100",
//	    "summary": "...",
//	    "image_url": "...",
//	    "year": "1997"
//	  },
//	  ...
//	]
func (s *SourceB) FetchAll(ctx context.Context) ([]models.MangaCanonical, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/titles", nil)
	if err != nil {
		return nil, fmt.Errorf("source_b: build request: %w", err)
	}

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("source_b: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("source_b: status %d: %s", resp.StatusCode, string(body))
	}

	// TODO: adapt to your actual JSON structure.
	var raw []struct {
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
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("source_b: decode json: %w", err)
	}

	result := make([]models.MangaCanonical, 0, len(raw))
	for _, r := range raw {
		if r.Slug == "" || r.Name == "" {
			continue
		}

		chapters := parseIntOrZero(r.TotalChapters)
		year := parseIntOrZero(r.Year)

		m := models.MangaCanonical{
			ID:            r.Slug,
			Title:         r.Name,
			AltTitles:     r.AltNames,
			Author:        r.Creator,
			Genres:        r.Tags,
			Status:        normalizeStatusB(r.State),
			TotalChapters: chapters,
			Description:   r.Summary,
			CoverURL:      r.ImageURL,
			Year:          year,
			SourceIDs:     map[string]string{"source_b": r.Slug},
		}
		result = append(result, m)
	}
	return result, nil
}

func parseIntOrZero(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func normalizeStatusB(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "completed", "finished", "end":
		return "completed"
	case "ongoing", "publishing", "running":
		return "ongoing"
	case "hiatus":
		return "hiatus"
	default:
		return s
	}
}
