package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	Path string
}

func DefaultConfig() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return Config{
		Path: filepath.Join(home, ".mangahub", "data.db"),
	}
}

func EnsureDataDir(cfg Config) error {
	return os.MkdirAll(filepath.Dir(cfg.Path), 0o755)
}

func Open(cfg Config) (*sql.DB, error) {
	if err := EnsureDataDir(cfg); err != nil {
		return nil, fmt.Errorf("ensure data dir: %w", err)
	}

	db, err := sql.Open("sqlite3", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	return db, nil
}

func MustOpen(cfg Config) *sql.DB {
	db, err := Open(cfg)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	return db
}
