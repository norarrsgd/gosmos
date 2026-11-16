// Package tuner stores query statistics and recommends observed configurations.
package tuner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Hints struct {
	MaxConcurrency      int `json:"maxConcurrency"`
	ContinuationLimitKB int `json:"continuationLimitKB"`

	PageSize   int `json:"pageSize"`
	BufferSize int `json:"bufferSize"`
}
type Sample struct {
	Duration  time.Duration `json:"duration"`
	Count     int           `json:"count"`
	Hash      string        `json:"hash"`
	RU        float64       `json:"ru"`
	Success   bool          `json:"success"`
	Timestamp time.Time     `json:"timestamp"`
	Strategy  string        `json:"strategy"`
	Hints     Hints         `json:"hints"`
}
type Storage interface {
	Record(context.Context, Sample) error
	Samples(context.Context, string, time.Time) ([]Sample, error)
}
type Memory struct {
	mu      sync.Mutex
	samples []Sample
}

func NewMemory() *Memory { return &Memory{} }
func validate(s Sample) error {
	if s.Hash == "" || s.Timestamp.IsZero() || math.IsNaN(s.RU) || math.IsInf(s.RU, 0) || s.RU < 0 {
		return fmt.Errorf("invalid query sample")
	}
	return nil
}
func (m *Memory) Record(ctx context.Context, s Sample) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validate(s); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, s)
	if len(m.samples) > 1000 {
		m.samples = append([]Sample(nil), m.samples[len(m.samples)-1000:]...)
	}
	return nil
}
func filter(rows []Sample, hash string, since time.Time) []Sample {
	out := []Sample{}
	for _, s := range rows {
		if s.Hash == hash && !s.Timestamp.Before(since) {
			out = append(out, s)
		}
	}
	return out
}
func (m *Memory) Samples(ctx context.Context, hash string, since time.Time) ([]Sample, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return filter(m.samples, hash, since), nil
}

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

type Recommendation struct {
	Strategy string
	Hints    Hints
}

func Recommend(ctx context.Context, s Storage, hash string, now time.Time) (Recommendation, error) {
	rows, err := s.Samples(ctx, hash, now.Add(-24*time.Hour))
	if err != nil {
		return Recommendation{}, err
	}
	type stats struct {
		n, ok int
		ru    float64
	}
	strategies := map[string]stats{}
	hints := map[Hints]stats{}
	count := 0
	for _, v := range rows {
		if v.Timestamp.After(now) {
			continue
		}
		count++
		a := strategies[v.Strategy]
		a.n++
		a.ru += v.RU
		if v.Success {
			a.ok++
		}
		strategies[v.Strategy] = a
		b := hints[v.Hints]
		b.n++
		b.ru += v.RU
		if v.Success {
			b.ok++
		}
		hints[v.Hints] = b
	}
	if count < 10 {
		return Recommendation{}, nil
	}
	var r Recommendation
	var best stats
	for k, a := range strategies {
		if a.n < 5 || k == "" {
			continue
		}
		if best.n == 0 || float64(a.ok)/float64(a.n) > float64(best.ok)/float64(best.n) || float64(a.ok)/float64(a.n) == float64(best.ok)/float64(best.n) && (a.ru/float64(a.n) < best.ru/float64(best.n) || a.ru/float64(a.n) == best.ru/float64(best.n) && k < r.Strategy) {
			r.Strategy = k
			best = a
		}
	}
	best = stats{}
	for k, a := range hints {
		if a.n < 3 {
			continue
		}
		if best.n == 0 || a.ru/float64(a.n) < best.ru/float64(best.n) || a.ru/float64(a.n) == best.ru/float64(best.n) && (float64(a.ok)/float64(a.n) > float64(best.ok)/float64(best.n) || float64(a.ok)/float64(a.n) == float64(best.ok)/float64(best.n) && (k.PageSize < r.Hints.PageSize || k.PageSize == r.Hints.PageSize && k.BufferSize < r.Hints.BufferSize)) {
			r.Hints = k
			best = a
		}
	}
	return r, nil
}

// Defaults supplies conservative shape-based hints; SDK compatibility knobs may be no-ops.
func Defaults(kind string) Hints {
	switch kind {
	case "vector", "fulltext", "hybrid":
		return Hints{PageSize: 50, BufferSize: 50}
	case "aggregation":
		return Hints{PageSize: 500, BufferSize: 1000}
	case "complex":
		return Hints{MaxConcurrency: 1, ContinuationLimitKB: 1}
	default:
		return Hints{}
	}
}
