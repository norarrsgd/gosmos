// Darkmatter demonstrates the eighteen Universe scenarios. It generates SQL by
// default; only an explicit -live flag permits Cosmos requests.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/norarrsgd/gosmos"
	"github.com/norarrsgd/gosmos/query"
	"github.com/norarrsgd/gosmos/tuner"
)

type product struct {
	gosmos.Base
	TenantID    string  `json:"tenantId" gosmos:"pk=1"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Price       float64 `json:"price"`
	Quantity    int     `json:"quantity"`
}
type productSummary struct {
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Category string  `json:"category"`
	Computed string  `json:"-"`
}
type configuration struct{ endpoint, key, database, container, partition string }

func offlineConfiguration() configuration {
	return configuration{endpoint: "https://localhost:8081", key: base64.StdEncoding.EncodeToString(make([]byte, 64)), database: "examples", container: "products", partition: "demo"}
}
func (c configuration) validate() error {
	if strings.TrimSpace(c.endpoint) == "" || strings.TrimSpace(c.key) == "" || strings.TrimSpace(c.database) == "" || strings.TrimSpace(c.container) == "" || strings.TrimSpace(c.partition) == "" {
		return errors.New("-live requires GOSMOS_ENDPOINT, GOSMOS_KEY, GOSMOS_DATABASE, GOSMOS_CONTAINER, and GOSMOS_PARTITION")
	}
	return nil
}

func main() {
	number := flag.Int("example", 18, "DarkMatter example number, 1..18")
	all := flag.Bool("all", false, "generate all examples offline")
	live := flag.Bool("live", false, "execute against explicitly configured Cosmos account; examples 5, 6, 17 write data")
	flag.Parse()
	if *all && *live {
		log.Fatal("-all is offline only; choose one -example for -live")
	}
	cfg := offlineConfiguration()
	if *live {
		cfg = configuration{endpoint: os.Getenv("GOSMOS_ENDPOINT"), key: os.Getenv("GOSMOS_KEY"), database: os.Getenv("GOSMOS_DATABASE"), container: os.Getenv("GOSMOS_CONTAINER"), partition: os.Getenv("GOSMOS_PARTITION")}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if *all {
		for n := 1; n <= 18; n++ {
			if e := runExample(ctx, os.Stdout, n, false, cfg); e != nil {
				log.Fatal(e)
			}
		}
		return
	}
	if e := runExample(ctx, os.Stdout, *number, *live, cfg); e != nil {
		log.Fatal(e)
	}
}

var names = []string{"", "Basic paged query", "Aggregates with GROUP BY", "TOP and DISTINCT", "Complex filtering", "Single item operations and cache", "Bulk operations", "Advanced aggregation", "Sales analysis", "Vector search", "Full-text search", "Hybrid vector and full-text", "Query generation", "Query optimization", "SQL injection protection", "Contains operator", "Typed projection selection", "Atomic operations", "Query capabilities"}

func runExample(ctx context.Context, out io.Writer, number int, live bool, cfg configuration) error {
	if number < 1 || number > 18 {
		return errors.New("example must be 1..18")
	}
	if e := cfg.validate(); e != nil {
		return e
	}
	credential, e := azcosmos.NewKeyCredential(cfg.key)
	if e != nil {
		return e
	}
	client, e := azcosmos.NewClientWithKey(cfg.endpoint, credential, nil)
	if e != nil {
		return e
	}
	history := tuner.NewMemory()
	galaxy, e := gosmos.New[product](client, cfg.database, cfg.container, gosmos.WithCache(100, time.Minute), gosmos.WithTuner(history))
	if e != nil {
		return e
	}
	fmt.Fprintf(out, "\n%02d: %s\n", number, names[number])
	orbit := galaxy.Query().InPartition(cfg.partition)
	if live {
		switch number {
		case 5:
			return singleItem(ctx, out, galaxy, cfg.partition)
		case 6:
			return bulkItems(ctx, out, galaxy, cfg.partition)
		case 17:
			return atomicItem(ctx, out, galaxy, cfg.partition)
		}
	}
	switch number {
	case 1:
		orbit.Cluster(func(c *gosmos.ClusterBuilder) { c.Eq("category", "Electronics") }).OrderBy("name")
		fmt.Fprintln(out, "Page size: 10; continuation tokens stay opaque.")
	case 2:
		orbit.Select("category").Aggregate("id", query.Count).GroupBy("category")
	case 3:
		orbit.Select("category").Distinct().Top(5).OrderBy("category")
	case 4:
		orbit.Cluster(func(c *gosmos.ClusterBuilder) { c.Eq("category", "Electronics").Or().Eq("category", "Books") }).Cluster(func(c *gosmos.ClusterBuilder) { c.Gte("price", 10).Lt("price", 100).Defined("description") })
	case 5:
		orbit.Cluster(func(c *gosmos.ClusterBuilder) { c.Eq("name", "Gosmos example") }).Top(1)
		fmt.Fprintln(out, "Offline preview; -live creates, reads twice with cache, replaces with ETag, and deletes one synthetic document.")
	case 6:
		orbit.Select("id", "name").Cluster(func(c *gosmos.ClusterBuilder) { c.StartsWith("name", "Gosmos bulk") })
		fmt.Fprintln(out, "Offline preview; -live creates three documents using Bulk and removes confirmed successful creations.")
	case 7:
		orbit.Select("category").Aggregate("price", query.Sum).Aggregate("price", query.Avg).Aggregate("quantity", query.Sum).Aggregate("id", query.Count)
	case 8:
		orbit.Select("category").Aggregate("price", query.Sum).Aggregate("quantity", query.Sum).Aggregate("id", query.Count).Cluster(func(c *gosmos.ClusterBuilder) { c.Gte("addedOn", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) })
	case 9:
		orbit.Select("id", "name").Top(5).Cluster(func(c *gosmos.ClusterBuilder) { c.VectorDistance("embedding", []float64{0.1, 0.2, 0.3}) })
	case 10:
		orbit.Select("id", "name").Cluster(func(c *gosmos.ClusterBuilder) {
			c.FTContains("description", "wireless").FTContainsAny("description", []string{"bluetooth", "portable"})
		})
	case 11:
		orbit.Select("id", "name").Top(5).Cluster(func(c *gosmos.ClusterBuilder) {
			c.VectorDistance("embedding", []float64{0.1, 0.2, 0.3}).FTScore("description", []string{"wireless", "portable"})
		}).WithWeights(0.7, 0.3)
	case 12:
		orbit.Select("id", "name", "price").Top(10).Cluster(func(c *gosmos.ClusterBuilder) { c.Eq("category", "Electronics").Gte("price", 50) }).OrderByDescending("price")
	case 13:
		orbit.Select("name", "price").Top(10).Cluster(func(c *gosmos.ClusterBuilder) { c.Eq("category", "Electronics") }).WithHints(gosmos.Hints{Strategy: gosmos.SinglePartition, MaxItemCount: 100})
		fmt.Fprintf(out, "Vector defaults: %+v; live execution records actual observations in memory.\n", tuner.Defaults("vector"))
	case 14:
		for _, column := range []string{`name"; DROP --`, `name] OR true`, `name/*comment*/`} {
			if _, e := query.Build(query.Input{Columns: query.ColumnOptions{Names: []string{column}}}); e == nil {
				return errors.New("unsafe identifier accepted")
			}
		}
		fmt.Fprintln(out, "Three malicious identifiers rejected before execution; the following payload is bound as literal data.")
		orbit.Cluster(func(c *gosmos.ClusterBuilder) { c.Eq("name", `test'); DROP TABLE c; --`) })
	case 15:
		orbit.Select("id", "category").Cluster(func(c *gosmos.ClusterBuilder) {
			c.Contains("category", []string{"Electronics", "Books"}).NotContains("name", []string{"discontinued"})
		})
	case 16:
		orbit = gosmos.SelectAs[productSummary](orbit).Top(5).Cluster(func(c *gosmos.ClusterBuilder) { c.Gt("quantity", 0) })
	case 17:
		patch := new(gosmos.Patch[product])
		if _, e := patch.Increment("quantity", 1).Build(); e != nil {
			return e
		}
		orbit.Select("id", "quantity").Top(1)
		fmt.Fprintln(out, "Validated increment patch; -live creates a synthetic document, atomically replaces with ETag, then atomically patches with the new ETag, and deletes it.")
	case 18:
		orbit.Cluster(func(c *gosmos.ClusterBuilder) {
			c.StartsWith("tenantId", "tenant:").StringContains("data.name", "sample", query.IgnoreCase).StringEquals("data.owner", "user", query.IgnoreCase).IsString("data.name").IsNull("data.deletedAt").ArrayContains("data.shares", map[string]any{"key": "user"}, query.PartialMatch).Compare(query.ArrayLength("data.shareEvents"), query.Gt, query.Field("data.auditQueued"))
		}).Project(func(p *query.ProjectionBuilder) {
			p.Field("id", "id").Object("data", func(p *query.ProjectionBuilder) { p.Field("name", "data.name").Field("owner", "data.owner") })
		}).WithConsistency(azcosmos.ConsistencyLevelStrong)
	}
	statement, e := orbit.GenerateQuery()
	if e != nil {
		return e
	}
	fmt.Fprintln(out, statement.Text)
	fmt.Fprintf(out, "Bound parameters: %d\n", len(statement.Params))
	if !live {
		fmt.Fprintln(out, "SQL generation only; no Cosmos requests. Synthetic example output is not a production logging standard.")
		return nil
	}
	if number == 1 {
		rows, gravity, e := orbit.Paged(ctx, query.Page{Size: 10})
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "Rows: %d; RU: %.2f; next page available: %t\n", len(rows), gravity.RU, gravity.ContinuationToken != "")
		if gravity.ContinuationToken != "" {
			rows, gravity, e = orbit.Paged(ctx, query.Page{Size: 10, ContinuationToken: gravity.ContinuationToken})
			if e != nil {
				return e
			}
			fmt.Fprintf(out, "Next rows: %d; RU: %.2f\n", len(rows), gravity.RU)
		}
		return nil
	}
	if number == 16 {
		rows, gravity, e := gosmos.ListAs[productSummary](ctx, orbit)
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "Typed rows: %d; RU: %.2f\n", len(rows), gravity.RU)
		return nil
	}
	rows, gravity, e := gosmos.ListAs[map[string]any](ctx, orbit)
	if e != nil {
		return e
	}
	fmt.Fprintf(out, "Rows: %d; RU: %.2f\n", len(rows), gravity.RU)
	if gravity.StatisticsError != nil {
		return gravity.StatisticsError
	}
	if number == 13 {
		normalized, _ := query.Normalize(statement.Text)
		recommendation, err := tuner.Recommend(ctx, history, fmt.Sprintf("%x", sha256.Sum256([]byte(normalized))), time.Now())
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Observed recommendation: %+v; at least ten samples are needed for data-driven recommendations.\n", recommendation)
	}
	return nil
}
