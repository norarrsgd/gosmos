package exec

import (
	"github.com/norarrsgd/gosmos/query"
	"testing"
)

func TestGuard(t *testing.T) {
	for _, shape := range []query.Shape{{OrderBy: true}, {Top: true}, {Distinct: true}, {Aggregate: true}, {GroupBy: true}, {Vector: true}, {FullText: true}, {RRF: true}} {
		if e := Guard(shape, false, true); e == nil {
			t.Fatal("unguarded shape", shape)
		}
		if e := Guard(shape, true, true); e != nil {
			t.Fatal(e)
		}
	}
	if e := Guard(query.Shape{}, false, true); e != nil {
		t.Fatal(e)
	}
	if e := Guard(query.Shape{}, false, false); e == nil {
		t.Fatal("cross partition disabled")
	}
}
