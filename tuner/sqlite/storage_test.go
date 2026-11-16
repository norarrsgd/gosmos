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
