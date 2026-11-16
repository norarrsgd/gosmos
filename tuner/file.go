package tuner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// File serializes access within this process; use one process per statistics file.
type File struct{ path string }

type fileLock struct {
	slot chan struct{}
	refs int
	path string
	info os.FileInfo
}

// locks serializes each statistics file; entries exist only while a caller holds or awaits the lock.
// Callers share a lock when they name the same file on disk (case, hard-link, or normalization aliases)
// or, for a file not yet created, the same path ignoring case.
var locks struct {
	sync.Mutex
	active []*fileLock
}

func (f *File) lock(ctx context.Context) (func(), error) {
	info, _ := os.Stat(f.path)
	locks.Lock()
	var l *fileLock
	for _, a := range locks.active {
		if info != nil && a.info != nil && os.SameFile(info, a.info) || strings.EqualFold(a.path, f.path) {
			l = a
			break
		}
	}
	if l == nil {
		l = &fileLock{slot: make(chan struct{}, 1), path: f.path}
		locks.active = append(locks.active, l)
	}
	if l.info == nil {
		l.info = info
	}
	l.refs++
	locks.Unlock()
	release := func() {
		locks.Lock()
		if l.refs--; l.refs == 0 {
			locks.active = slices.DeleteFunc(locks.active, func(a *fileLock) bool { return a == l })
		}
		locks.Unlock()
	}
	select {
	case l.slot <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-l.slot
			release()
			return nil, err
		}
		return func() { <-l.slot; release() }, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
}

func NewFile(path string) (*File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("statistics path must be absolute and clean")
	}
	info, err := os.Lstat(path)
	if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return nil, fmt.Errorf("statistics file must be regular and owner-only")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// Resolving the parent directory gives aliases of one file (symlinked directories) the same lock.
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	return &File{filepath.Join(dir, filepath.Base(path))}, nil
}

func (f *File) read() ([]Sample, error) {
	b, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []Sample
	if err = json.Unmarshal(b, &rows); err != nil {
		backup, backupErr := os.CreateTemp(filepath.Dir(f.path), ".gosmos-corrupt-*")
		if backupErr != nil {
			return nil, backupErr
		}
		if backupErr = backup.Close(); backupErr != nil {
			return nil, backupErr
		}
		if backupErr = os.Rename(f.path, backup.Name()); backupErr != nil {
			os.Remove(backup.Name())
			return nil, backupErr
		}
		return nil, nil
	}
	return rows, nil
}

func (f *File) Samples(ctx context.Context, hash string, since time.Time) ([]Sample, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	unlock, err := f.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	rows, err := f.read()
	return filter(rows, hash, since), err
}

func (f *File) Record(ctx context.Context, s Sample) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validate(s); err != nil {
		return err
	}
	unlock, err := f.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	rows, err := f.read()
	if err != nil {
		return err
	}
	rows = append(rows, s)
	if len(rows) > 1000 {
		rows = rows[len(rows)-1000:]
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".gosmos-stats-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	// No fsync: statistics are advisory, and read() moves a truncated file aside instead of failing.
	return os.Rename(tmp.Name(), f.path)
}
