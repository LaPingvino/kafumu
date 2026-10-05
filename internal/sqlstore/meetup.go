package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/meetup"
)

// Meetups is meetup.Store on SQLite.
type Meetups struct{ DB *sql.DB }

var _ meetup.Store = (*Meetups)(nil)

// meetupRow is meetup.Meetup with every field in its JSON (the public JSON
// leaves out the author, RSVPs and expiry). Same fields, other tags: a
// field added to Meetup breaks the conversion at compile time, so it can't
// be silently lost here.
type meetupRow struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Business   string    `json:"business,omitempty"`
	Via        string    `json:"via,omitempty"`
	ATURI      string    `json:"at_uri,omitempty"`
	ATCID      string    `json:"at_cid,omitempty"`
	Title      string    `json:"title"`
	Text       string    `json:"text,omitempty"`
	StartAt    time.Time `json:"start_at"`
	EndAt      time.Time `json:"end_at"`
	Venue      string    `json:"venue,omitempty"`
	Link       string    `json:"link,omitempty"`
	Cell       string    `json:"cell"`
	Tags       []string  `json:"tags,omitempty"`
	RSVPs      []string  `json:"rsvps,omitempty"`
	Going      int       `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// querier is what both *sql.DB and *sql.Tx offer.
type querier interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

func putMeetup(ctx context.Context, q querier, m *meetup.Meetup) error {
	b, err := json.Marshal(meetupRow(*m))
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT OR REPLACE INTO meetups (id, cell, author_id, expires_at, data) VALUES (?, ?, ?, ?, ?)`,
		m.ID, m.Cell, m.AuthorID, m.ExpiresAt.Unix(), string(b))
	return err
}

func queryMeetups(ctx context.Context, q querier, query string, args ...any) ([]*meetup.Meetup, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*meetup.Meetup
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var r meetupRow
		if err := json.Unmarshal([]byte(data), &r); err != nil {
			return nil, err
		}
		m := meetup.Meetup(r)
		out = append(out, &m)
	}
	return out, rows.Err()
}

func getMeetup(ctx context.Context, q querier, id string) (*meetup.Meetup, error) {
	ms, err := queryMeetups(ctx, q, `SELECT data FROM meetups WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		return nil, meetup.ErrNotFound
	}
	return ms[0], nil
}

func (s *Meetups) Get(ctx context.Context, id string) (*meetup.Meetup, error) {
	return getMeetup(ctx, s.DB, id)
}

func (s *Meetups) Put(ctx context.Context, m *meetup.Meetup) error { return putMeetup(ctx, s.DB, m) }

func (s *Meetups) Delete(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM meetups WHERE id = ?`, id)
	return err
}

func (s *Meetups) InCells(ctx context.Context, cells []string, now time.Time) ([]*meetup.Meetup, error) {
	if len(cells) == 0 {
		return nil, nil
	}
	args := []any{now.Unix()}
	for _, c := range cells {
		args = append(args, c)
	}
	return queryMeetups(ctx, s.DB, `SELECT data FROM meetups WHERE expires_at > ? AND cell IN (?`+strings.Repeat(",?", len(cells)-1)+`)`, args...)
}

func (s *Meetups) ActiveBy(ctx context.Context, authorID string, now time.Time) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM meetups WHERE author_id = ? AND expires_at > ?`, authorID, now.Unix()).Scan(&n)
	return n, err
}

func (s *Meetups) Update(ctx context.Context, id string, fn func(*meetup.Meetup) error) (*meetup.Meetup, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	m, err := getMeetup(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := fn(m); err != nil {
		return nil, err
	}
	if err := putMeetup(ctx, tx, m); err != nil {
		return nil, err
	}
	return m, tx.Commit()
}

// PurgeMeetups deletes expired meetups (the self-hosted purge).
func (s *Meetups) PurgeMeetups(ctx context.Context, now time.Time) (int64, error) {
	r, err := s.DB.ExecContext(ctx, `DELETE FROM meetups WHERE expires_at <= ?`, now.Unix())
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}
