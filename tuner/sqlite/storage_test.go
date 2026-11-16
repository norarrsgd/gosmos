package sqlite

import (
	"context"
	"github.com/norarrsgd/gosmos/tuner"
	"path/filepath"
	"testing"
	"time"
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
	path := filepath.Join(t.TempDir(), "stats?mode=memory.db")
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
