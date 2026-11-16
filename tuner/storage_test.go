package tuner

import (
	"context"
	"errors"
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
func TestFileLockWaitHonorsContext(t *testing.T) {
	f, e := NewFile(filepath.Join(t.TempDir(), "stats.json"))
	if e != nil {
		t.Fatal(e)
	}
	unlock, e := f.lock(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e = f.Record(ctx, Sample{Hash: "q", Timestamp: time.Now()}); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	other, e := NewFile(filepath.Join(t.TempDir(), "other.json"))
	if e != nil || other.Record(context.Background(), Sample{Hash: "q", Timestamp: time.Now()}) != nil {
		t.Fatal("a held lock blocked a different file", e)
	}
}
func TestFileAliasesShareLockAndEntriesAreReleased(t *testing.T) {
	real := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if e := os.Symlink(real, alias); e != nil {
		t.Fatal(e)
	}
	a, e := NewFile(filepath.Join(real, "stats.json"))
	if e != nil {
		t.Fatal(e)
	}
	b, e := NewFile(filepath.Join(alias, "stats.json"))
	if e != nil {
		t.Fatal(e)
	}
	unlock, e := a.lock(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e = b.Record(ctx, Sample{Hash: "q", Timestamp: time.Now()}); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("alias acquired a separate lock:", e)
	}
	unlock()
	locks.Lock()
	defer locks.Unlock()
	if len(locks.active) != 0 {
		t.Fatal(len(locks.active))
	}
}

func TestFileLockRejectsCancelledContext(t *testing.T) {
	f, err := NewFile(filepath.Join(t.TempDir(), "stats.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 128 {
		unlock, err := f.lock(ctx)
		if unlock != nil {
			unlock()
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled context acquired the lock: %v", err)
		}
	}
	locks.Lock()
	defer locks.Unlock()
	if len(locks.active) != 0 {
		t.Fatal("cancelled lock retained entries:", len(locks.active))
	}
}

func TestFileKeepsResolvedPathAfterParentSymlinkChanges(t *testing.T) {
	original, replacement := t.TempDir(), t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(original, alias); err != nil {
		t.Fatal(err)
	}
	f, err := NewFile(filepath.Join(alias, "stats.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(replacement, alias); err != nil {
		t.Fatal(err)
	}
	if err := f.Record(context.Background(), Sample{Hash: "q", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	originalStore, err := NewFile(filepath.Join(original, "stats.json"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := originalStore.Samples(context.Background(), "q", time.Time{})
	if err != nil || len(rows) != 1 {
		t.Fatal("sample was not written to the original directory:", rows, err)
	}
	if _, err := os.Stat(filepath.Join(replacement, "stats.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("record followed the changed parent symlink:", err)
	}
}
func TestFileAliasesOnDiskShareLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")
	f, e := NewFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Record(context.Background(), Sample{Hash: "q", Timestamp: time.Now()}); e != nil {
		t.Fatal(e)
	}
	aliases := []string{filepath.Join(dir, "hard.json")}
	if e = os.Link(path, aliases[0]); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(dir, "STATS.JSON")); e == nil { // case-insensitive filesystem
		aliases = append(aliases, filepath.Join(dir, "STATS.JSON"))
	}
	unlock, e := f.lock(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	for _, alias := range aliases {
		g, e := NewFile(alias)
		if e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		e = g.Record(ctx, Sample{Hash: "q", Timestamp: time.Now()})
		cancel()
		if !errors.Is(e, context.DeadlineExceeded) {
			t.Fatalf("%s acquired a separate lock: %v", alias, e)
		}
	}
}
