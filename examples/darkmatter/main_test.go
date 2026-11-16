package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAllExamplesGenerateWithoutRequests(t *testing.T) {
	cfg := configuration{endpoint: "https://localhost:1", key: base64.StdEncoding.EncodeToString(make([]byte, 64)), database: "examples", container: "products", partition: "demo"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for n := 1; n <= 18; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			var output bytes.Buffer
			if e := runExample(ctx, &output, n, false, cfg); e != nil {
				t.Fatal(e)
			}
			if output.Len() == 0 {
				t.Fatal("no example output")
			}
			if n == 18 && (!strings.Contains(output.String(), "STARTSWITH") || !strings.Contains(output.String(), "Bound parameters: 4")) {
				t.Fatal(output.String())
			}
		})
	}
}
func TestLiveConfigurationRequired(t *testing.T) {
	for _, n := range []int{1, 5, 6, 17, 18} {
		if e := runExample(context.Background(), new(bytes.Buffer), n, true, configuration{}); e == nil {
			t.Fatalf("example %d accepted missing credentials", n)
		}
	}
}
func TestUnknownExample(t *testing.T) {
	if e := runExample(context.Background(), new(bytes.Buffer), 19, false, offlineConfiguration()); e == nil {
		t.Fatal("unknown example accepted")
	}
}
