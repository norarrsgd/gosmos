package gosmos

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestQueryHasNoExternalDependencies(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-json", "./query")
	out, e := cmd.Output()
	if e != nil {
		t.Fatal(e)
	}
	d := json.NewDecoder(strings.NewReader(string(out)))
	for d.More() {
		var p struct {
			ImportPath string
			Standard   bool
		}
		if e = d.Decode(&p); e != nil {
			t.Fatal(e)
		}
		if !p.Standard && p.ImportPath != "github.com/norarrsgd/gosmos/query" {
			t.Fatal("query imports", p.ImportPath)
		}
	}
}
