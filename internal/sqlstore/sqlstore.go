// Package sqlstore keeps Kafumu's data in SQLite, for running outside App
// Engine (self-hosting, forks): set KAFUMU_SQLITE=/path/to/kafumu.db. It
// implements the same store interfaces as the Datastore and memory stores.
// The driver is pure Go (modernc.org/sqlite): no C compiler needed.
package sqlstore

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) the database and its tables.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: one writer; simple and safe
	for _, s := range schema {
		if _, err := db.Exec(s); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlstore: schema: %w", err)
		}
	}
	return db, nil
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS notes (
		id TEXT PRIMARY KEY, cell TEXT, pair TEXT, asks TEXT, expires_at INTEGER, data TEXT NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS notes_cell ON notes (cell, expires_at)`,
	`CREATE INDEX IF NOT EXISTS notes_pair ON notes (pair)`,
	`CREATE TABLE IF NOT EXISTS hidden_notes (id TEXT PRIMARY KEY, at INTEGER)`,
}
