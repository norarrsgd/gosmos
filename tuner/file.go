package tuner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// File serializes access within this process; use one process per statistics file.
type File struct{ path string }

var fileMu sync.Mutex

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
	return &File{path}, nil
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
	fileMu.Lock()
	defer fileMu.Unlock()
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
	fileMu.Lock()
	defer fileMu.Unlock()
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
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.path)
}
