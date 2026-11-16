// Package sqlite provides optional SQLite query-statistics storage.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/norarrsgd/gosmos/tuner"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

type Storage struct{ db *sql.DB }

func Open(path string) (*Storage, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("statistics path must be absolute and clean")
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e == nil {
		if e = f.Close(); e != nil {
			return nil, e
		}
	} else if !os.IsExist(e) {
		return nil, e
	}
	info, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("statistics file must be regular and owner-only")
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	if _, e = db.Exec(`CREATE TABLE IF NOT EXISTS samples (hash TEXT NOT NULL, timestamp INTEGER NOT NULL, value BLOB NOT NULL)`); e != nil {
		db.Close()
		return nil, e
	}
	return &Storage{db}, nil
}
func (s *Storage) Close() error { return s.db.Close() }
func (s *Storage) Record(ctx context.Context, v tuner.Sample) error {
	m := tuner.NewMemory()
	if e := m.Record(ctx, v); e != nil {
		return e
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO samples(hash,timestamp,value) VALUES(?,?,?)`, v.Hash, v.Timestamp.UnixNano(), b)
	return e
}
func (s *Storage) Samples(ctx context.Context, hash string, since time.Time) ([]tuner.Sample, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT value FROM samples WHERE hash=? AND timestamp>=? ORDER BY timestamp`, hash, since.UnixNano())
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []tuner.Sample{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var v tuner.Sample
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
