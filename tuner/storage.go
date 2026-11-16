// Package tuner stores query statistics and recommends observed configurations.
package tuner

import (
	"context"
	"fmt"
	"math"
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

func validate(s Sample) error {
	if s.Hash == "" || s.Timestamp.IsZero() || math.IsNaN(s.RU) || math.IsInf(s.RU, 0) || s.RU < 0 {
		return fmt.Errorf("invalid query sample")
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
