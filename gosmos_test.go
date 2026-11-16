package gosmos_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/norarrsgd/gosmos"
	"github.com/norarrsgd/gosmos/query"
	"github.com/norarrsgd/gosmos/tuner"
)

type document struct {
	gosmos.Base
	Tenant string `json:"tenant" gosmos:"pk=1"`
	Name   string `json:"name"`
}

func TestRootAPI(t *testing.T) {
	credential, err := azcosmos.NewKeyCredential("a2V5")
	if err != nil {
		t.Fatal(err)
	}
	client, err := azcosmos.NewClientWithKey("https://example.documents.azure.com", credential, nil)
	if err != nil {
		t.Fatal(err)
	}
	galaxy, err := gosmos.New[document](client, "db", "items",
		gosmos.WithNamingPolicy(strings.ToLower),
		gosmos.WithCache(10, time.Minute),
		gosmos.WithTuner(tuner.NewMemory()),
		gosmos.WithConsistency(azcosmos.ConsistencyLevelSession),
		gosmos.WithRecordQueries(true),
		gosmos.WithCrossPartition(false),
		gosmos.WithAutoProvision(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		Name string `json:"name"`
	}
	orbit := gosmos.SelectAs[result](galaxy.Query().InPartition("tenant").Cluster(func(c *gosmos.ClusterBuilder) {
		c.StringEquals("Name", "sample", gosmos.IgnoreCase).In("Tenant", []string{"tenant"}, gosmos.PartialMatch)
	}).WithHints(gosmos.Hints{Strategy: gosmos.SinglePartition}))
	statement, err := orbit.GenerateQuery()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(statement.Text, `SELECT c["name"] FROM c WHERE `) || len(statement.Params) != 2 {
		t.Fatalf("unexpected statement: %#v", statement)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = gosmos.ListAs[result](ctx, orbit); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, _, err = gosmos.PagedAs[result](ctx, orbit, query.Page{Size: 10}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, _, err = gosmos.GetAs[result](ctx, orbit); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = galaxy.Atomic("tenant").Patch("id", func(p *gosmos.Patch[document]) {
		p.Set("name", "updated")
	}, gosmos.IfMatch("etag")).Execute(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestRootErrorIdentity(t *testing.T) {
	_, err := gosmos.New[document](nil, "db", "items")
	var operation *gosmos.Error
	if !errors.Is(err, gosmos.ErrValidation) || !errors.As(err, &operation) {
		t.Fatalf("root error types lost: %v", err)
	}
}
