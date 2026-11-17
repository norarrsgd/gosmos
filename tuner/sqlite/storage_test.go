package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/norarrsgd/gosmos/tuner"
)

func TestRoundTrip(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "stats.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now()
	if e = s.Record(context.Background(), tuner.Sample{Hash: "q", RU: 3, Timestamp: now}); e != nil {
		t.Fatal(e)
	}
	rows, e := s.Samples(context.Background(), "q", now.Add(-time.Hour))
	if e != nil || len(rows) != 1 || rows[0].RU != 3 {
		t.Fatal(rows, e)
	}
}
func TestPathWithQueryCharacters(t *testing.T) {
	for _, name := range []string{"stats?mode=memory.db", "stats?_pragma=query_only(1).db", "stats#fragment.db", "stats%3Fencoded.db", "stats with spaces & unicode-雪.db"} {
		t.Run(name, func(t *testing.T) {
			testLiteralDatabasePath(t, name)
		})
	}
}
func testLiteralDatabasePath(t *testing.T, name string) {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, name)
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Record(context.Background(), tuner.Sample{Hash: "q", Timestamp: time.Now()}); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	rows, e := s.Samples(context.Background(), "q", time.Time{})
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	entries, e := os.ReadDir(directory)
	if e != nil || len(entries) != 1 || entries[0].Name() != name {
		t.Fatal("expected only the literal database filename:", entries, e)
	}
}
func TestSecureParentSymlink(t *testing.T) {
	directory := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if e := os.Symlink(directory, alias); e != nil {
		t.Fatal(e)
	}
	s, e := Open(filepath.Join(alias, "stats.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Record(context.Background(), tuner.Sample{Hash: "q", Timestamp: time.Now()}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(directory, "stats.db")); e != nil {
		t.Fatal(e)
	}
}
func TestRejectsWritableParentBeforeCreatingDatabase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses retained handles instead of Unix directory modes")
	}
	directory := t.TempDir()
	if e := os.Chmod(directory, 0777); e != nil {
		t.Fatal(e)
	}
	defer os.Chmod(directory, 0700)
	path := filepath.Join(directory, "stats.db")
	if s, e := Open(path); e == nil {
		s.Close()
		t.Fatal("opened a database in a replaceable directory")
	}
	if _, e := os.Lstat(path); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("database was created before rejecting its directory:", e)
	}
}
func TestRejectsReplaceableParentSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses retained handles instead of Unix directory modes")
	}
	directory := t.TempDir()
	replaceable := t.TempDir()
	if e := os.Chmod(replaceable, 0777); e != nil {
		t.Fatal(e)
	}
	defer os.Chmod(replaceable, 0700)
	alias := filepath.Join(replaceable, "alias")
	if e := os.Symlink(directory, alias); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(alias, "stats.db")
	if s, e := Open(path); e == nil {
		s.Close()
		t.Fatal("opened a database through a replaceable symlink")
	}
	if _, e := os.Lstat(filepath.Join(directory, "stats.db")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("database was created before rejecting the symlink:", e)
	}
}
func TestRejectsDatabaseSymlinkWithoutTouchingTarget(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.db")
	contents := []byte("not a database")
	if e := os.WriteFile(target, contents, 0600); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(directory, "stats.db")
	if e := os.Symlink(target, path); e != nil {
		t.Fatal(e)
	}
	if s, e := Open(path); e == nil {
		s.Close()
		t.Fatal("opened a database through a file symlink")
	}
	after, e := os.ReadFile(target)
	if e != nil || string(after) != string(contents) {
		t.Fatal("symlink target changed:", string(after), e)
	}
}
func TestRetentionKeepsNewestSamples(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "stats.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	base := time.Now()
	for i := range MaxSamples + 5 {
		if e = s.Record(context.Background(), tuner.Sample{Hash: "q", RU: float64(i), Timestamp: base.Add(time.Duration(i))}); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := s.Samples(context.Background(), "q", time.Time{})
	if e != nil || len(rows) != MaxSamples || rows[0].RU != 5 {
		t.Fatal(len(rows), e)
	}
}
func TestFailedPruneRollsBackInsert(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "stats.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, statement := range []string{
		`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<10000) INSERT INTO samples SELECT 'q', i, x'7b7d' FROM n`,
		`CREATE TRIGGER fail_prune BEFORE DELETE ON samples BEGIN SELECT RAISE(ABORT, 'prune failed'); END`,
	} {
		if _, e = s.db.Exec(statement); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Record(context.Background(), tuner.Sample{Hash: "q", Timestamp: time.Now()}); e == nil {
		t.Fatal("record succeeded although pruning failed")
	}
	var count int
	if e = s.db.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&count); e != nil || count != MaxSamples {
		t.Fatal(count, e)
	}
}
