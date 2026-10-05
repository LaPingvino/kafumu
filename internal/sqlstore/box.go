package sqlstore

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/box"
	"github.com/LaPingvino/kafumu/internal/slot"
)

// Boxes is box.Store on SQLite: one row per message; a message older than
// box.TTL is gone (unlisted at once, deleted by PurgeBoxes).
type Boxes struct{ DB *sql.DB }

var _ box.Store = (*Boxes)(nil)

func (s *Boxes) Append(ctx context.Context, id string, m box.Message) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM box_msgs WHERE box = ? AND at > ?`, id, time.Now().Add(-box.TTL).UnixNano()).Scan(&n); err != nil {
		return err
	}
	if n >= box.MaxMessages {
		return box.ErrFull
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO box_msgs (box, id, data, at) VALUES (?, ?, ?, ?)`, id, m.ID, m.Data, m.At.UnixNano()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Boxes) List(ctx context.Context, id string) ([]box.Message, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, data, at FROM box_msgs WHERE box = ? AND at > ? ORDER BY at`, id, time.Now().Add(-box.TTL).UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []box.Message
	for rows.Next() {
		var m box.Message
		var at int64
		if err := rows.Scan(&m.ID, &m.Data, &at); err != nil {
			return nil, err
		}
		m.At = time.Unix(0, at).UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Boxes) Ack(ctx context.Context, id string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	args := []any{id}
	for _, x := range ids {
		args = append(args, x)
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM box_msgs WHERE box = ? AND id IN (?`+strings.Repeat(",?", len(ids)-1)+`)`, args...)
	return err
}

// PurgeBoxes deletes messages past box.TTL (the self-hosted purge).
func (s *Boxes) PurgeBoxes(ctx context.Context, now time.Time) (int64, error) {
	r, err := s.DB.ExecContext(ctx, `DELETE FROM box_msgs WHERE at <= ?`, now.Add(-box.TTL).UnixNano())
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

// Slots is slot.Store on SQLite.
type Slots struct{ DB *sql.DB }

var _ slot.Store = (*Slots)(nil)

func (s *Slots) Get(ctx context.Context, id string) ([]string, error) {
	var tokens string
	err := s.DB.QueryRowContext(ctx, `SELECT tokens FROM slots WHERE id = ? AND expires_at > ?`, id, time.Now().Unix()).Scan(&tokens)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil || tokens == "" {
		return nil, err
	}
	return strings.Split(tokens, "\n"), nil
}

func (s *Slots) Put(ctx context.Context, id string, tokens []string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO slots (id, tokens, expires_at) VALUES (?, ?, ?)`,
		id, strings.Join(tokens, "\n"), time.Now().Add(slot.TTL).Unix())
	return err
}

// PurgeSlots deletes expired slots.
func (s *Slots) PurgeSlots(ctx context.Context, now time.Time) (int64, error) {
	r, err := s.DB.ExecContext(ctx, `DELETE FROM slots WHERE expires_at <= ?`, now.Unix())
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}
