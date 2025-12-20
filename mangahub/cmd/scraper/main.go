package main

import (
	"context"
	"log"
	"time"

	"mangahub/internal/scraper"
	"mangahub/pkg/database"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db := database.MustOpen(database.DefaultConfig())
	defer db.Close()

	// Ensure schema exists
	if err := database.Migrate(db); err != nil {
		log.Fatalf("db migrate failed: %v", err)
	}

	// Source A: MangaDex (live)
	srcA := scraper.NewSourceA()

	// Source B: local mirror server (demo-safe)
	srcB := scraper.NewSourceB("http://localhost:9000")

	agg := scraper.NewAggregator(srcA, srcB)

	mangas, err := agg.FetchAndMerge(ctx)
	if err != nil {
		log.Fatalf("scrape failed: %v", err)
	}

	log.Printf("merged mangas: %d", len(mangas))

	if err := scraper.SaveToDatabase(ctx, db, mangas); err != nil {
		log.Fatalf("save failed: %v", err)
	}

	log.Println("✅ database populated at ~/.mangahub/data.db")
}
