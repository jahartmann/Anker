package anker

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
)

type Store struct{ db *sql.DB }

func OpenStore(p string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", p)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA synchronous=FULL; CREATE TABLE IF NOT EXISTS records (bucket TEXT NOT NULL, id TEXT NOT NULL, value BLOB NOT NULL, PRIMARY KEY(bucket,id));`)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = os.Chmod(p, 0600); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Put(bucket, id string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO records(bucket,id,value) VALUES(?,?,?) ON CONFLICT(bucket,id) DO UPDATE SET value=excluded.value`, bucket, id, b)
	return err
}
func (s *Store) Get(bucket, id string, v any) error {
	var b []byte
	err := s.db.QueryRow(`SELECT value FROM records WHERE bucket=? AND id=?`, bucket, id).Scan(&b)
	if err != nil {
		return fmt.Errorf("%s %s: %w", bucket, id, err)
	}
	return json.Unmarshal(b, v)
}
func (s *Store) Delete(bucket, id string) error {
	_, err := s.db.Exec(`DELETE FROM records WHERE bucket=? AND id=?`, bucket, id)
	return err
}
func records[T any](s *Store, bucket string) ([]T, error) {
	rows, err := s.db.Query(`SELECT value FROM records WHERE bucket=? ORDER BY id`, bucket)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var b []byte
		var v T
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
