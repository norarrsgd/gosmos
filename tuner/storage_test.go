package tuner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorageAndRecommendations(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	s := NewMemory()
	for i := 0; i < 10; i++ {
		if err := s.Record(ctx, Sample{Hash: "q", RU: 2, Success: true, Timestamp: now, Strategy: "single", Hints: Hints{PageSize: 50}}); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Recommend(ctx, s, "q", now)
	if err != nil || r.Strategy != "single" || r.Hints.PageSize != 50 {
		t.Fatal(r, err)
	}
	f, err := NewFile(filepath.Join(t.TempDir(), "stats.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Record(ctx, Sample{Hash: "q", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.Samples(ctx, "q", now.Add(-time.Hour))
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	info, _ := os.Stat(f.path)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
}
func TestInsufficientSamples(t *testing.T) {
	r, e := Recommend(context.Background(), NewMemory(), "q", time.Now())
	if e != nil || r.Strategy != "" {
		t.Fatal(r, e)
	}
}
func TestFileRecoversCorruptionAndBoundsHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	os.WriteFile(path, []byte("broken"), 0600)
	f, e := NewFile(path)
	if e != nil {
		t.Fatal(e)
	}
	rows, e := f.Samples(context.Background(), "q", time.Time{})
	if e != nil || len(rows) != 0 {
		t.Fatal(rows, e)
	}
	for range 1001 {
		if e = f.Record(context.Background(), Sample{Hash: "q", Timestamp: time.Now()}); e != nil {
			t.Fatal(e)
		}
	}
	rows, e = f.Samples(context.Background(), "q", time.Time{})
	if e != nil || len(rows) != 1000 {
		t.Fatal(len(rows), e)
	}
}
func TestDefaults(t *testing.T) {
	h := Defaults("vector")
	if h.PageSize != 50 || h.BufferSize != 50 {
		t.Fatal(h)
	}
	h = Defaults("aggregation")
	if h.PageSize != 500 || h.BufferSize != 1000 {
		t.Fatal(h)
	}
}
