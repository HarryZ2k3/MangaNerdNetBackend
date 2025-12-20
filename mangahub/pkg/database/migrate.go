package database

import (
	"database/sql"
	"fmt"
	"os"
)

func Migrate(db *sql.DB) error {
	b, err := os.ReadFile("docs/schema.sql")
	if err != nil {
		return fmt.Errorf("read docs/schema.sql: %w", err)
	}

	if _, err := db.Exec(string(b)); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}
