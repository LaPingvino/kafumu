package sqlstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"time"

	"github.com/LaPingvino/kafumu/internal/purge"
)

// Purge is the self-hosted counterpart of purge.Run (App Engine cron plus
// Datastore's TTL policy): it deletes what Kafumu promised to forget, by
// the same rules.
func Purge(ctx context.Context, db *sql.DB, now time.Time) (string, error) {
	var n struct{ notes, meetups, boxes, slots, small, users int64 }
	var err error
	if n.notes, err = (&Notes{DB: db}).PurgeNotes(ctx, now); err != nil {
		return "", err
	}
	if n.meetups, err = (&Meetups{DB: db}).PurgeMeetups(ctx, now); err != nil {
		return "", err
	}
	if n.boxes, err = (&Boxes{DB: db}).PurgeBoxes(ctx, now); err != nil {
		return "", err
	}
	if n.slots, err = (&Slots{DB: db}).PurgeSlots(ctx, now); err != nil {
		return "", err
	}
	if n.small, err = purgeKV(ctx, db, now); err != nil {
		return "", err
	}
	if n.users, err = purgeUsers(ctx, db, now); err != nil {
		return "", err
	}
	return fmt.Sprintf("notes=%d meetups=%d box-messages=%d slots=%d small=%d users=%d", n.notes, n.meetups, n.boxes, n.slots, n.small, n.users), nil
}

// purgeKV drops expired entries of the kinds that carry an ExpiresAt
// (decoded on its own: gob skips the other fields), and expired hides.
func purgeKV(ctx context.Context, db *sql.DB, now time.Time) (int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT kind, id, data FROM kv WHERE kind IN ('Handle', 'ShortCode', 'Report', 'PushSub', 'Hidden')`)
	if err != nil {
		return 0, err
	}
	type key struct{ kind, id string }
	var gone []key
	for rows.Next() {
		var k key
		var data []byte
		if err := rows.Scan(&k.kind, &k.id, &data); err != nil {
			rows.Close()
			return 0, err
		}
		var exp time.Time
		if k.kind == "Hidden" {
			err = gob.NewDecoder(bytes.NewReader(data)).Decode(&exp)
		} else {
			var v struct{ ExpiresAt time.Time }
			err = gob.NewDecoder(bytes.NewReader(data)).Decode(&v)
			exp = v.ExpiresAt
		}
		if err == nil && !exp.IsZero() && !exp.After(now) {
			gone = append(gone, k)
		}
	}
	rows.Close()
	for _, k := range gone {
		if _, err := db.ExecContext(ctx, `DELETE FROM kv WHERE kind = ? AND id = ?`, k.kind, k.id); err != nil {
			return 0, err
		}
	}
	return int64(len(gone)), nil
}

// purgeUsers deletes idle accounts by purge's rules, with their sync vault
// and username.
func purgeUsers(ctx context.Context, db *sql.DB, now time.Time) (int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, last_seen_at, data FROM users WHERE last_seen_at < ?`, now.Add(-24*time.Hour).Unix())
	if err != nil {
		return 0, err
	}
	type victim struct{ id, name string }
	var gone []victim
	for rows.Next() {
		var id, data string
		var seen int64
		if err := rows.Scan(&id, &seen, &data); err != nil {
			rows.Close()
			return 0, err
		}
		var u struct {
			Username string
			KeepDays int
		}
		if json.Unmarshal([]byte(data), &u) != nil {
			continue
		}
		keep := purge.AnonymousIdle
		if u.Username != "" {
			keep = purge.NamedIdle
			if u.KeepDays > 0 {
				keep = time.Duration(u.KeepDays) * 24 * time.Hour
			}
		}
		if u.KeepDays == -1 || now.Sub(time.Unix(seen, 0)) < keep {
			continue
		}
		gone = append(gone, victim{id, u.Username})
	}
	rows.Close()
	for _, v := range gone {
		for _, q := range []struct {
			sql  string
			args []any
		}{{`DELETE FROM users WHERE id = ?`, []any{v.id}}, {`DELETE FROM vaults WHERE user = ?`, []any{v.id}},
			{`DELETE FROM usernames WHERE name = ? AND owner = ?`, []any{v.name, v.id}}} {
			if _, err := db.ExecContext(ctx, q.sql, q.args...); err != nil {
				return 0, err
			}
		}
	}
	return int64(len(gone)), nil
}
