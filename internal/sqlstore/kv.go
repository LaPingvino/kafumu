package sqlstore

import "database/sql"

// KV is kv.Persister on SQLite: one table of (kind, id, gob bytes).
type KV struct{ DB *sql.DB }

func (s *KV) LoadKind(kind string) (map[string][]byte, error) {
	rows, err := s.DB.Query(`SELECT id, data FROM kv WHERE kind = ?`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		out[id] = data
	}
	return out, rows.Err()
}

func (s *KV) SaveOne(kind, id string, data []byte) error {
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO kv (kind, id, data) VALUES (?, ?, ?)`, kind, id, data)
	return err
}

func (s *KV) DeleteOne(kind, id string) error {
	_, err := s.DB.Exec(`DELETE FROM kv WHERE kind = ? AND id = ?`, kind, id)
	return err
}
