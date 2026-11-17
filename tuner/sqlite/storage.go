// Package sqlite provides optional SQLite query-statistics storage.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/norarrsgd/gosmos/tuner"
	_ "modernc.org/sqlite"
)

// MaxSamples is the number of newest samples kept across all query hashes.
const MaxSamples = 10000

type Storage struct {
	db        *sql.DB
	directory *os.File
	file      *os.File
}

func Open(path string) (*Storage, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("statistics path must be absolute and clean")
	}
	directoryPath, directoryInfo, e := secureDirectory(filepath.Dir(path))
	if e != nil {
		return nil, e
	}
	directory, e := os.Open(directoryPath)
	if e != nil {
		return nil, e
	}
	closeDirectory := true
	defer func() {
		if closeDirectory {
			directory.Close()
		}
	}()
	openedDirectoryInfo, e := directory.Stat()
	if e != nil {
		return nil, e
	}
	if !os.SameFile(directoryInfo, openedDirectoryInfo) {
		return nil, fmt.Errorf("statistics directory changed while opening")
	}

	path = filepath.Join(directoryPath, filepath.Base(path))
	f, e := openDatabaseFile(path)
	if e != nil {
		return nil, e
	}
	closeFile := true
	defer func() {
		if closeFile {
			f.Close()
		}
	}()

	// Escape path characters so the driver cannot interpret them as DSN options.
	dsn := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	db, e := sql.Open("sqlite", dsn.String())
	if e != nil {
		return nil, e
	}
	closeDatabase := true
	defer func() {
		if closeDatabase {
			db.Close()
		}
	}()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if e = db.Ping(); e != nil {
		return nil, e
	}
	if e = verifyLiveDatabase(db, path, f); e != nil {
		return nil, e
	}
	if _, e = db.Exec(`CREATE TABLE IF NOT EXISTS samples (hash TEXT NOT NULL, timestamp INTEGER NOT NULL, value BLOB NOT NULL)`); e != nil {
		return nil, e
	}
	if _, e = db.Exec(`CREATE INDEX IF NOT EXISTS samples_hash_timestamp ON samples(hash, timestamp)`); e != nil {
		return nil, e
	}
	closeDatabase = false
	closeDirectory = false
	closeFile = false
	return &Storage{db: db, directory: directory, file: f}, nil
}

func openDatabaseFile(path string) (*os.File, error) {
	info, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		f, createErr := openFileNoFollow(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if createErr == nil {
			if e = validateDatabaseFile(f); e != nil {
				f.Close()
				return nil, e
			}
			return f, nil
		}
		if !errors.Is(createErr, os.ErrExist) {
			return nil, createErr
		}
		info, e = os.Lstat(path)
	}
	if e != nil {
		return nil, e
	}
	if e = validateDatabaseInfo(info); e != nil {
		return nil, e
	}
	f, e := openFileNoFollow(path, os.O_RDWR, 0)
	if e != nil {
		return nil, e
	}
	openedInfo, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	if !os.SameFile(info, openedInfo) {
		f.Close()
		return nil, fmt.Errorf("statistics file changed while opening")
	}
	if e = validateDatabaseInfo(openedInfo); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}

func validateDatabaseFile(f *os.File) error {
	info, e := f.Stat()
	if e != nil {
		return e
	}
	return validateDatabaseInfo(info)
}

func validateDatabaseInfo(info os.FileInfo) error {
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !ownedByProcess(info) {
		return fmt.Errorf("statistics file must be a regular owner-only file owned by the current user")
	}
	return nil
}

func verifyLiveDatabase(db *sql.DB, path string, expected *os.File) error {
	expectedInfo, e := expected.Stat()
	if e != nil {
		return e
	}
	pathInfo, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if e = validateDatabaseInfo(pathInfo); e != nil || !os.SameFile(expectedInfo, pathInfo) {
		if e != nil {
			return e
		}
		return fmt.Errorf("statistics file changed before database connection")
	}

	rows, e := db.Query(`PRAGMA database_list`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var sequence int
		var name, livePath string
		if e = rows.Scan(&sequence, &name, &livePath); e != nil {
			return e
		}
		if name != "main" {
			continue
		}
		liveInfo, statErr := os.Stat(livePath)
		if statErr != nil {
			return statErr
		}
		if e = validateDatabaseInfo(liveInfo); e != nil {
			return e
		}
		if !os.SameFile(expectedInfo, liveInfo) {
			return fmt.Errorf("SQLite opened a different statistics file")
		}
		return nil
	}
	if e = rows.Err(); e != nil {
		return e
	}
	return fmt.Errorf("SQLite did not report a main statistics database")
}

func (s *Storage) Close() error {
	return errors.Join(s.db.Close(), s.file.Close(), s.directory.Close())
}
func (s *Storage) Record(ctx context.Context, v tuner.Sample) error {
	m := tuner.NewMemory()
	if e := m.Record(ctx, v); e != nil {
		return e
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	// One transaction, so cancellation never leaves an inserted row unpruned.
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(ctx, `INSERT INTO samples(hash,timestamp,value) VALUES(?,?,?)`, v.Hash, v.Timestamp.UnixNano(), b)
	if e != nil {
		return e
	}
	// Rowids grow monotonically because only the oldest rows are deleted.
	last, e := result.LastInsertId()
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM samples WHERE rowid<=?`, last-MaxSamples); e != nil {
		return e
	}
	return tx.Commit()
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
