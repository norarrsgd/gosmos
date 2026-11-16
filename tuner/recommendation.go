package tuner

import (
	"context"
	"time"
)

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
