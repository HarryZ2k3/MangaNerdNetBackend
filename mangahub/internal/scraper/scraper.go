package scraper

import (
	"context"
	"log"
	"mangahub/pkg/models"
	"strings"
	"unicode"
)

// Source is implemented by each external data source (API / HTML / local mirror).
// Each source is responsible for fetching its own data format and mapping it
// into MangaCanonical.
type Source interface {
	Name() string
	FetchAll(ctx context.Context) ([]models.MangaCanonical, error)
}

// Aggregator coordinates calls to multiple sources and merges them into a single
// canonical set of manga entries.
type Aggregator struct {
	Sources []Source
}

// NewAggregator creates a new Aggregator with the given sources.
func NewAggregator(sources ...Source) *Aggregator {
	return &Aggregator{Sources: sources}
}

// FetchAndMerge fetches all manga from all sources and merges them
// into a single slice of MangaCanonical using deterministic conflict
// resolution rules.
func (a *Aggregator) FetchAndMerge(ctx context.Context) ([]models.MangaCanonical, error) {
	byKey := make(map[string]models.MangaCanonical)

	for _, src := range a.Sources {
		log.Printf("[scraper] fetching from %s", src.Name())
		mangas, err := src.FetchAll(ctx)
		if err != nil {
			log.Printf("[scraper] source %s error: %v", src.Name(), err)
			// keep going: one broken source should not kill all scraping
			continue
		}

		for _, m := range mangas {
			key := canonicalKey(m)

			if existing, ok := byKey[key]; ok {
				merged := mergeManga(existing, m, src.Name())
				byKey[key] = merged
			} else {
				byKey[key] = m
			}
		}
	}

	result := make([]models.MangaCanonical, 0, len(byKey))
	for _, m := range byKey {
		result = append(result, m)
	}
	return result, nil
}

// canonicalKey defines how we group entries that represent the “same manga”
// coming from different sources. For now we use a normalized title key.
// You can refine this later (e.g. prefer a primary source ID).
func canonicalKey(m models.MangaCanonical) string {
	return normalizeKey(m.Title)
}

// normalizeKey converts a string to a canonical form: lowercase,
// remove non-letter/digit characters and compress spaces.
func normalizeKey(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))

	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		// treat everything else as space separator
		if !prevSpace {
			b.WriteRune(' ')
			prevSpace = true
		}
	}
	// final trim
	return strings.TrimSpace(b.String())
}

// mergeManga defines our conflict resolution rules when two sources
// describe the same manga. The idea:
//
// - Keep base.Title as the canonical title, but add incoming.Title to AltTitles if different.
// - Fill missing fields from incoming.
// - Merge genres (set union).
// - For status: if any says "completed", use "completed".
// - For total chapters: use the maximum.
// - For description: keep whichever is longer.
// - For cover: keep existing; if empty, use incoming.
// - Merge SourceIDs.
func mergeManga(base, incoming models.MangaCanonical, sourceName string) models.MangaCanonical {
	// Alt titles
	if incoming.Title != "" && incoming.Title != base.Title {
		base.AltTitles = appendIfMissing(base.AltTitles, incoming.Title)
	}

	// Author
	if base.Author == "" && incoming.Author != "" {
		base.Author = incoming.Author
	}

	// Genres (set union)
	base.Genres = mergeStringSlices(base.Genres, incoming.Genres)

	// Status resolution
	base.Status = resolveStatus(base.Status, incoming.Status)

	// Total chapters: prefer the higher number
	if incoming.TotalChapters > base.TotalChapters {
		base.TotalChapters = incoming.TotalChapters
	}

	// Description: prefer the longer one
	if len(incoming.Description) > len(base.Description) {
		base.Description = incoming.Description
	}

	// Cover: prefer existing; if empty, use incoming
	if base.CoverURL == "" && incoming.CoverURL != "" {
		base.CoverURL = incoming.CoverURL
	}

	// Year: if base has 0 and incoming has >0, use incoming
	if base.Year == 0 && incoming.Year > 0 {
		base.Year = incoming.Year
	}

	// Merge SourceIDs
	if base.SourceIDs == nil {
		base.SourceIDs = make(map[string]string)
	}
	for k, v := range incoming.SourceIDs {
		base.SourceIDs[k] = v
	}

	return base
}

func appendIfMissing(slice []string, v string) []string {
	for _, x := range slice {
		if x == v {
			return slice
		}
	}
	return append(slice, v)
}

func mergeStringSlices(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	out = append(out, a...)
	for _, v := range b {
		out = appendIfMissing(out, v)
	}
	return out
}

// resolveStatus merges two status values with a simple rule:
// - If either is "completed", return "completed".
// - Else if one is non-empty, use that.
// - Else empty.
func resolveStatus(a, b string) string {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))

	if a == "completed" || b == "completed" {
		return "completed"
	}
	if a != "" {
		return a
	}
	return b
}
