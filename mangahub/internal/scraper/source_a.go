package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mangahub/pkg/models"
)

// MangaDex API base (public)
const mangadexBase = "https://api.mangadex.org"

// SourceA fetches manga list from MangaDex.
type SourceA struct {
	Client *http.Client
	Limit  int // items per request
	Max    int // maximum items to fetch total (safety)
}

func NewSourceA() *SourceA {
	return &SourceA{
		Client: &http.Client{Timeout: 12 * time.Second},
		Limit:  50,
		Max:    200, // keep demo-safe; bump later if you want
	}
}

func (s *SourceA) Name() string { return "mangadex" }

type mdResponse struct {
	Result string `json:"result"`
	Data   []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			Title       map[string]string   `json:"title"`
			AltTitles   []map[string]string `json:"altTitles"`
			Description map[string]string   `json:"description"`
			Status      string              `json:"status"`
			Year        int                 `json:"year"`
			Tags        []struct {
				Attributes struct {
					Name map[string]string `json:"name"`
				} `json:"attributes"`
			} `json:"tags"`
		} `json:"attributes"`
		Relationships []struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes struct {
				Name     string `json:"name"`     // author
				FileName string `json:"fileName"` // cover_art
			} `json:"attributes"`
		} `json:"relationships"`
	} `json:"data"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

func (s *SourceA) FetchAll(ctx context.Context) ([]models.MangaCanonical, error) {
	var all []models.MangaCanonical

	offset := 0
	fetched := 0

	for fetched < s.Max {
		u, _ := url.Parse(mangadexBase + "/manga")
		q := u.Query()
		q.Set("limit", fmt.Sprintf("%d", s.Limit))
		q.Set("offset", fmt.Sprintf("%d", offset))

		// keep results “safe” and more consistent for demo
		q.Add("contentRating[]", "safe")
		q.Add("contentRating[]", "suggestive")

		// include author + cover data in relationships
		q.Add("includes[]", "author")
		q.Add("includes[]", "cover_art")

		// optional: prefer English availability if you want (not required)
		// q.Add("availableTranslatedLanguage[]", "en")

		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("mangadex: build request: %w", err)
		}

		resp, err := s.Client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("mangadex: request: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("mangadex: status %d: %s", resp.StatusCode, string(body))
		}

		var md mdResponse
		if err := json.Unmarshal(body, &md); err != nil {
			return nil, fmt.Errorf("mangadex: decode: %w", err)
		}

		if len(md.Data) == 0 {
			break
		}

		for _, item := range md.Data {
			if item.ID == "" {
				continue
			}

			title := pickLang(item.Attributes.Title, "en")
			if title == "" {
				// fallback to any title
				for _, v := range item.Attributes.Title {
					title = v
					break
				}
			}
			if title == "" {
				continue
			}

			desc := pickLang(item.Attributes.Description, "en")

			genres := make([]string, 0, len(item.Attributes.Tags))
			for _, t := range item.Attributes.Tags {
				name := pickLang(t.Attributes.Name, "en")
				if name != "" {
					genres = append(genres, name)
				}
			}

			author := ""
			coverURL := ""
			altTitles := make([]string, 0, len(item.Attributes.AltTitles))

			for _, m := range item.Attributes.AltTitles {
				at := pickLang(m, "en")
				if at == "" {
					for _, v := range m {
						at = v
						break
					}
				}
				if at != "" && at != title {
					altTitles = appendIfMissing(altTitles, at)
				}
			}

			coverFile := ""
			for _, rel := range item.Relationships {
				switch rel.Type {
				case "author":
					if author == "" && rel.Attributes.Name != "" {
						author = rel.Attributes.Name
					}
				case "cover_art":
					if coverFile == "" && rel.Attributes.FileName != "" {
						coverFile = rel.Attributes.FileName
					}
				}
			}
			if coverFile != "" {
				coverURL = fmt.Sprintf("https://uploads.mangadex.org/covers/%s/%s", item.ID, coverFile)
			}

			m := models.MangaCanonical{
				ID:            item.ID, // canonical ID = MangaDex UUID
				Title:         title,
				AltTitles:     altTitles,
				Author:        author,
				Genres:        genres,
				Status:        normalizeStatusMD(item.Attributes.Status),
				TotalChapters: 0, // MangaDex doesn't directly give total chapters in this list endpoint
				Description:   desc,
				CoverURL:      coverURL,
				Year:          item.Attributes.Year,
				SourceIDs:     map[string]string{"mangadex": item.ID},
			}
			all = append(all, m)
			fetched++
			if fetched >= s.Max {
				break
			}
		}

		offset += s.Limit
	}

	return all, nil
}

func pickLang(m map[string]string, lang string) string {
	if m == nil {
		return ""
	}
	if v := strings.TrimSpace(m[lang]); v != "" {
		return v
	}
	return ""
}

func normalizeStatusMD(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ongoing":
		return "ongoing"
	case "completed":
		return "completed"
	case "hiatus":
		return "hiatus"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return strings.ToLower(strings.TrimSpace(s))
	}
}
