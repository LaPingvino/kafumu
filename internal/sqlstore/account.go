package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/account"
)

// Accounts is account.Store on SQLite: users as JSON with the queried
// fields (cell, visibility) in columns, and the username registry (shared
// with businesses, as "biz:<id>") in its own table.
type Accounts struct{ DB *sql.DB }

var _ account.Store = (*Accounts)(nil)

func (s *Accounts) Get(ctx context.Context, id string) (*account.User, error) {
	var data string
	err := s.DB.QueryRowContext(ctx, `SELECT data FROM users WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, account.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var u account.User
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		return nil, err
	}
	u.ID = id
	return &u, nil
}

func (s *Accounts) Put(ctx context.Context, u *account.User) error {
	b, err := json.Marshal(u)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO users (id, cell, visible_until, last_seen_at, data) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.Cell, u.VisibleUntil.Unix(), u.LastSeenAt.Unix(), string(b))
	return err
}

func (s *Accounts) Delete(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

// ClaimUsername reserves name for id in one statement: it inserts, or
// "updates" a row that is already id's; any other owner leaves it as is.
func (s *Accounts) ClaimUsername(ctx context.Context, name, id string) error {
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO usernames (name, owner) VALUES (?, ?) ON CONFLICT(name) DO NOTHING`, name, id); err != nil {
		return err
	}
	owner, err := s.LookupUsername(ctx, name)
	if err != nil {
		return err
	}
	if owner != id {
		return account.ErrTaken
	}
	return nil
}

func (s *Accounts) ReleaseUsername(ctx context.Context, name, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM usernames WHERE name = ? AND owner = ?`, name, id)
	return err
}

func (s *Accounts) LookupUsername(ctx context.Context, name string) (string, error) {
	var owner string
	err := s.DB.QueryRowContext(ctx, `SELECT owner FROM usernames WHERE name = ?`, name).Scan(&owner)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return owner, err
}

func (s *Accounts) VisibleIn(ctx context.Context, cells []string, now time.Time) ([]*account.User, error) {
	if len(cells) == 0 {
		return nil, nil
	}
	args := []any{now.Unix()}
	for _, c := range cells {
		args = append(args, c)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, data FROM users WHERE visible_until > ? AND cell IN (?`+strings.Repeat(",?", len(cells)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*account.User
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		var u account.User
		if err := json.Unmarshal([]byte(data), &u); err != nil {
			return nil, err
		}
		u.ID = id
		out = append(out, &u)
	}
	return out, rows.Err()
}
