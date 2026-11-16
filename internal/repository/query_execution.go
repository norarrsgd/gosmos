package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	execution "github.com/norarrsgd/gosmos/internal/exec"
	"github.com/norarrsgd/gosmos/query"
	"github.com/norarrsgd/gosmos/tuner"
)

type queryContainer interface {
	NewQueryItemsPager(string, azcosmos.PartitionKey, *azcosmos.QueryOptions) *runtime.Pager[azcosmos.QueryItemsResponse]
}

func executeQuery[R any, T Entity](ctx context.Context, o *Orbit[T], page *query.Page, single bool) (result []R, metadata Gravity, resultErr error) {
	var g Gravity
	fail := func(err error) ([]R, Gravity, error) { return nil, g, wrapError("query", o.g.metadata.name, err) }
	if e := ctx.Err(); e != nil {
		return fail(e)
	}
	if page != nil && (page.Size < 1 || page.Size > 1000) {
		return fail(validation("page size must be 1 to 1000"))
	}
	h := o.hints
	if h.Strategy != Auto && h.Strategy != SinglePartition && h.Strategy != Gateway {
		return fail(validation("unknown strategy"))
	}
	if h.Strategy == SinglePartition && len(o.pk) == 0 {
		return fail(validation("single-partition strategy requires partition key"))
	}
	if h.MaxItemCount < 0 || h.MaxItemCount > 1000 || h.ResponseContinuationTokenLimitInKB < 0 || h.ResponseContinuationTokenLimitInKB > 16 || h.MaxConcurrency < 0 || h.MaxBufferedItemCount < 0 {
		return fail(validation("invalid hints"))
	}
	if page != nil && h != (Hints{}) {
		return fail(validation("paging and hints are mutually exclusive"))
	}
	level := o.read.ConsistencyLevel
	if level == nil {
		level = o.g.options.consistency
	}
	level, e := copyConsistency(level)
	if e != nil {
		return fail(e)
	}
	s, e := o.GenerateQuery()
	if e != nil {
		return fail(e)
	}
	if e = execution.Guard(s.Shape, len(o.pk) > 0, o.g.options.crossPartition); e != nil {
		return fail(errors.Join(ErrUnsupportedCrossPartition, e))
	}
	key := azcosmos.NewPartitionKey()
	if len(o.pk) > 0 {
		key, e = stringKey(o.pk, len(o.g.metadata.fields))
		if e != nil {
			return fail(e)
		}
	}
	opts := &azcosmos.QueryOptions{ConsistencyLevel: level, PageSizeHint: h.MaxItemCount, ResponseContinuationTokenLimitInKB: h.ResponseContinuationTokenLimitInKB}
	if page != nil {
		opts.PageSizeHint = int32(page.Size)
		if page.ContinuationToken != "" {
			token := page.ContinuationToken
			opts.ContinuationToken = &token
		}
	}
	if single {
		opts.PageSizeHint = 1
	}
	for _, p := range s.Params {
		opts.QueryParameters = append(opts.QueryParameters, azcosmos.QueryParameter{Name: p.Name, Value: p.Value})
	}
	if o.g.options.recordQueries {
		g.Query = &QueryInfo{Text: s.Text}
		for _, p := range s.Params {
			g.Query.Params = append(g.Query.Params, QueryParameter{Name: p.Name, Value: p.Value})
		}
	}
	if single && level == nil && o.g.options.cache != nil {
		normalized, _ := query.Normalize(s.Text)
		wire, marshalErr := json.Marshal([]any{"query", normalized, s.Params, o.pk, reflect.TypeFor[R]().PkgPath(), reflect.TypeFor[R]().String()})
		if marshalErr == nil {
			cacheKey := fmt.Sprintf("%x", sha256.Sum256(wire))
			version := o.g.options.cache.Version()
			if b, found := o.g.options.cache.Get(cacheKey); found {
				var cached []R
				if json.Unmarshal(b, &cached) == nil {
					return cached, g, nil
				}
				o.g.options.cache.Delete(cacheKey)
			}
			defer func() {
				if resultErr == nil {
					if b, e := json.Marshal(result); e == nil {
						o.g.options.cache.PutIfVersion(cacheKey, b, version)
					}
				}
			}()
		}
	}
	if o.g.options.statistics != nil {
		started := time.Now()
		defer func() {
			normalized, _ := query.Normalize(s.Text)
			strategy := "Gateway"
			if len(o.pk) > 0 {
				strategy = "SinglePartition"
			}
			statsCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			defer cancel()
			metadata.StatisticsError = o.g.options.statistics.Record(statsCtx, tuner.Sample{Hash: fmt.Sprintf("%x", sha256.Sum256([]byte(normalized))), RU: metadata.RU, Success: resultErr == nil, Timestamp: started, Strategy: strategy, Hints: tuner.Hints{PageSize: int(opts.PageSizeHint)}, Duration: time.Since(started), Count: len(result)})
		}()
	}
	c, ok := o.g.container.(queryContainer)
	if !ok {
		return fail(validation("container does not support queries"))
	}
	pager := c.NewQueryItemsPager(s.Text, key, opts)
	rows := []R{}
	for pager.More() {
		response, err := pager.NextPage(ctx)
		g.RU += float64(response.RequestCharge)
		if err != nil {
			return fail(err)
		}
		g.ContinuationToken = ""
		if response.ContinuationToken != nil {
			g.ContinuationToken = *response.ContinuationToken
		}
		for _, item := range response.Items {
			var value R
			if err = json.Unmarshal(item, &value); err != nil {
				return fail(err)
			}
			rows = append(rows, value)
			if single {
				return rows, g, nil
			}
		}
		if page != nil && len(rows) > 0 && !s.Shape.GroupBy {
			return rows, g, nil
		}
	}
	return rows, g, nil
}
