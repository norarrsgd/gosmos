// Query demonstrates SQL generation without credentials or a Cosmos account.
package main

import (
	"fmt"
	"github.com/norarrsgd/gosmos/query"
	"log"
)

func main() {
	statement, err := query.Build(query.Input{Clusters: []query.Cluster{{Catalysts: []query.Catalyst{{Column: "status", Value: "open"}, {Column: "total", Operator: query.Gt, Value: 100}}}}, Columns: query.ColumnOptions{Top: 25}, Sorting: []query.Sorting{{Column: "addedOn", Direction: query.Desc}}})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(statement.Text)
}
