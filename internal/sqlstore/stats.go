package sqlstore

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Stat is one admin number (the admin page's Stats, without Datastore).
type Stat struct {
	Name string
	N    int
	Note string
}

// Stats counts what the admin page shows on App Engine, from SQLite.
func Stats(ctx context.Context, db *sql.DB, now time.Time) []Stat {
	count := func(q string, args ...any) int {
		n := -1
		if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
			return -1
		}
		return n
	}
	day := now.Add(-24 * time.Hour).Unix()
	return []Stat{
		{"Accounts", count(`SELECT COUNT(*) FROM users`), "all, named or not"},
		{"Named", count(`SELECT COUNT(*) FROM users WHERE json_extract(data, '$.Username') != ''`), "kept a year"},
		{"Active 24 h", count(`SELECT COUNT(*) FROM users WHERE last_seen_at > ?`, day), ""},
		{"Active 7 d", count(`SELECT COUNT(*) FROM users WHERE last_seen_at > ?`, now.Add(-7*24*time.Hour).Unix()), ""},
		{"Active 30 d", count(`SELECT COUNT(*) FROM users WHERE last_seen_at > ?`, now.Add(-30*24*time.Hour).Unix()), ""},
		{"Findable now", count(`SELECT COUNT(*) FROM users WHERE visible_until > ?`, now.Unix()), "public profiles"},
		{"Synced", count(`SELECT COUNT(*) FROM vaults`), "accounts with a device vault"},
		{"Meetups", count(`SELECT COUNT(*) FROM meetups WHERE expires_at > ?`, now.Unix()), "upcoming or running"},
		{"Local messages", count(`SELECT COUNT(*) FROM notes WHERE expires_at > ?`, now.Unix()), "OLN, unexpired"},
		{"Mailbox messages", count(`SELECT COUNT(*) FROM box_msgs`), "pairing, inboxes, moves"},
		{"Push", count(`SELECT COUNT(*) FROM kv WHERE kind = 'PushSub'`), "devices with notifications"},
	}
}

// FindUsers returns account ids for the admin search: a username prefix,
// or (empty) the first named accounts alphabetically.
func FindUsers(ctx context.Context, db *sql.DB, prefix string) []string {
	q, args := `SELECT id FROM users WHERE json_extract(data, '$.Username') != '' ORDER BY json_extract(data, '$.Username') LIMIT 50`, []any{}
	if prefix = strings.ToLower(prefix); prefix != "" {
		q, args = `SELECT id FROM users WHERE json_extract(data, '$.Username') LIKE ? ESCAPE '\' ORDER BY json_extract(data, '$.Username') LIMIT 300`,
			[]any{strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(prefix) + "%"}
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
