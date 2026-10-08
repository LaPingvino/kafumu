package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/oln"
)

// Notes is oln.Store on SQLite.
type Notes struct{ DB *sql.DB }

var _ oln.Store = (*Notes)(nil)

// noteRow is everything of a note, including what its JSON form leaves out.
type noteRow struct {
	oln.Note
	Asks []string  `json:"asks,omitempty"`
	Pair string    `json:"pair,omitempty"`
	Re   string    `json:"re,omitempty"`
	Recv time.Time `json:"recv,omitempty"`
}

func (s *Notes) Put(ctx context.Context, n *oln.Note) error {
	b, err := json.Marshal(noteRow{Note: *n, Asks: n.Asks, Pair: n.Pair, Re: n.Re, Recv: n.Recv})
	if err != nil {
		return err
	}
	asks := ""
	if len(n.Asks) > 0 {
		asks = "," + strings.Join(n.Asks, ",") + ","
	}
	_, err = s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO notes (id, cell, pair, asks, expires_at, data) VALUES (?, ?, ?, ?, ?, ?)`,
		n.ID, n.Cell, n.Pair, asks, n.ExpiresAt.Unix(), string(b))
	return err
}

func scanNotes(rows *sql.Rows) ([]*oln.Note, error) {
	defer rows.Close()
	var out []*oln.Note
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var r noteRow
		if err := json.Unmarshal([]byte(data), &r); err != nil {
			return nil, err
		}
		n := r.Note
		n.Asks, n.Pair, n.Re, n.Recv = r.Asks, r.Pair, r.Re, r.Recv
		out = append(out, &n)
	}
	return out, rows.Err()
}

func (s *Notes) Get(ctx context.Context, id string) (*oln.Note, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM notes WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	ns, err := scanNotes(rows)
	if err != nil {
		return nil, err
	}
	if len(ns) == 0 {
		return nil, errors.New("sqlstore: note not found")
	}
	return ns[0], nil
}

func (s *Notes) InCells(ctx context.Context, cells []string, now time.Time) ([]*oln.Note, error) {
	if len(cells) == 0 {
		return nil, nil
	}
	q := `SELECT data FROM notes WHERE expires_at > ? AND cell IN (?` + strings.Repeat(",?", len(cells)-1) + `)`
	args := []any{now.Unix()}
	for _, c := range cells {
		args = append(args, c)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return scanNotes(rows)
}

func (s *Notes) AskedAbout(ctx context.Context, tag string, now time.Time) ([]*oln.Note, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM notes WHERE expires_at > ? AND asks LIKE ? LIMIT 50`, now.Unix(), "%,"+tag+",%")
	if err != nil {
		return nil, err
	}
	return scanNotes(rows)
}

func (s *Notes) ByPair(ctx context.Context, tag string, now time.Time) ([]*oln.Note, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM notes WHERE pair = ? AND expires_at > ? LIMIT 200`, tag, now.Unix())
	if err != nil {
		return nil, err
	}
	return scanNotes(rows)
}

func (s *Notes) Hidden(ctx context.Context) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM hidden_notes LIMIT 5000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	h := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		h[id] = true
	}
	return h, rows.Err()
}

func (s *Notes) Hide(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO hidden_notes (id, at) VALUES (?, ?)`, id, time.Now().Unix())
	return err
}

// PurgeNotes deletes expired notes (the self-hosted purge).
func (s *Notes) PurgeNotes(ctx context.Context, now time.Time) (int64, error) {
	r, err := s.DB.ExecContext(ctx, `DELETE FROM notes WHERE expires_at <= ?`, now.Unix())
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

// RepliesTo reads the re out of each row's JSON: no column, no migration
// (a self-hosted node's notes table is small).
func (s *Notes) RepliesTo(ctx context.Context, res []string, now time.Time) ([]*oln.Note, error) {
	if len(res) == 0 {
		return nil, nil
	}
	args := []any{now.Unix()}
	for _, re := range res {
		args = append(args, re)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM notes WHERE expires_at > ? AND json_extract(data, '$.re') IN (?`+strings.Repeat(", ?", len(res)-1)+`) LIMIT 300`, args...)
	if err != nil {
		return nil, err
	}
	return scanNotes(rows)
}
