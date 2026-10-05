package sqlstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/LaPingvino/kafumu/internal/vault"
)

// Vaults is vault.Store on SQLite: one encrypted blob per account, written
// only on top of the version the device last saw (optimistic, like
// Datastore's transaction).
type Vaults struct{ DB *sql.DB }

var _ vault.Store = (*Vaults)(nil)

func (s *Vaults) Get(ctx context.Context, user string) (*vault.Vault, error) {
	var v vault.Vault
	var at int64
	err := s.DB.QueryRowContext(ctx, `SELECT data, version, updated_at FROM vaults WHERE user = ?`, user).Scan(&v.Data, &v.Version, &at)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.UpdatedAt = time.Unix(0, at).UTC()
	return &v, nil
}

func (s *Vaults) Put(ctx context.Context, user string, want int, data []byte, now time.Time) (int, error) {
	if len(data) > vault.MaxBytes {
		return 0, vault.ErrTooBig
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	cur := 0
	if err := tx.QueryRowContext(ctx, `SELECT version FROM vaults WHERE user = ?`, user).Scan(&cur); err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if cur != want {
		return 0, vault.ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO vaults (user, data, version, updated_at) VALUES (?, ?, ?, ?)`,
		user, data, cur+1, now.UnixNano()); err != nil {
		return 0, err
	}
	return cur + 1, tx.Commit()
}

func (s *Vaults) Delete(ctx context.Context, user string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM vaults WHERE user = ?`, user)
	return err
}
