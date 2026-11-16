package tuner

import (
	"context"
	"sync"
	"time"
)

type Memory struct {
	mu      sync.Mutex
	samples []Sample
}

func NewMemory() *Memory { return &Memory{} }

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

func (m *Memory) Samples(ctx context.Context, hash string, since time.Time) ([]Sample, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return filter(m.samples, hash, since), nil
}
