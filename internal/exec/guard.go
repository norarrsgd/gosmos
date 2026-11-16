// Package exec holds Cosmos execution policy independently of repository types.
package exec

import (
	"errors"
	"fmt"
	"github.com/norarrsgd/gosmos/query"
	"strings"
)

var ErrUnsupportedCrossPartition = errors.New("gosmos: unsupported cross-partition query")

func Guard(s query.Shape, partitioned, crossPartition bool) error {
	if partitioned {
		return nil
	}
	if !crossPartition {
		return fmt.Errorf("%w: cross-partition execution disabled", ErrUnsupportedCrossPartition)
	}
	var shapes []string
	for _, v := range []struct {
		present bool
		name    string
	}{{s.OrderBy, "ORDER BY"}, {s.Top, "TOP"}, {s.Distinct, "DISTINCT"}, {s.Aggregate, "aggregate"}, {s.GroupBy, "GROUP BY"}, {s.Vector, "vector"}, {s.FullText, "full-text ranking"}, {s.RRF, "RRF"}} {
		if v.present {
			shapes = append(shapes, v.name)
		}
	}
	if len(shapes) > 0 {
		return fmt.Errorf("%w: %s", ErrUnsupportedCrossPartition, strings.Join(shapes, ", "))
	}
	return nil
}
