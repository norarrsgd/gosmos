// sync-corpus vendors an externally generated Universe corpus without changing its contents.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

type corpusCase struct {
	SchemaVersion  int             `json:"schemaVersion"`
	UniverseCommit string          `json:"universeCommit"`
	Name           string          `json:"name"`
	Tags           []string        `json:"tags"`
	Input          json.RawMessage `json:"input"`
	SQL            string          `json:"sql"`
	Params         json.RawMessage `json:"params"`
	ErrorCategory  string          `json:"errorCategory"`
}

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func syncCorpus(src, dst string) error {
	paths, e := filepath.Glob(filepath.Join(src, "*.json"))
	if e != nil {
		return e
	}
	if len(paths) == 0 {
		return errors.New("source contains no corpus cases")
	}
	sort.Strings(paths)
	data := map[string][]byte{}
	names := map[string]bool{}
	for _, path := range paths {
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		var c corpusCase
		if e = json.Unmarshal(b, &c); e != nil {
			return fmt.Errorf("%s: %w", path, e)
		}
		if c.SchemaVersion != 1 || !commitPattern.MatchString(c.UniverseCommit) || c.Name == "" || names[c.Name] || len(c.Input) == 0 || len(c.Tags) == 0 {
			return fmt.Errorf("%s: invalid corpus metadata or schema", path)
		}
		names[c.Name] = true
		reject := false
		for _, tag := range c.Tags {
			if tag == "reject" {
				reject = true
			}
		}
		if reject && c.ErrorCategory == "" || !reject && (c.SQL == "" || len(c.Params) == 0) {
			return fmt.Errorf("%s: expected result missing", path)
		}
		data[filepath.Base(path)] = b
	}
	if _, e = os.Stat(dst); !errors.Is(e, os.ErrNotExist) {
		return fmt.Errorf("destination must not exist; review corpus changes in a separate directory")
	}
	stage, e := os.MkdirTemp(filepath.Dir(dst), ".corpus-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	for name, b := range data {
		if e = os.WriteFile(filepath.Join(stage, name), b, 0644); e != nil {
			return e
		}
	}
	if e = os.Rename(stage, dst); e != nil {
		return e
	}
	fmt.Printf("validated %d cases into %s\n", len(data), dst)
	return nil
}
func main() {
	src := flag.String("source", "", "Universe corpus directory")
	dst := flag.String("destination", "testdata/corpus", "new destination directory")
	flag.Parse()
	if *src == "" {
		fmt.Fprintln(os.Stderr, "-source is required")
		os.Exit(2)
	}
	if e := syncCorpus(*src, *dst); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
