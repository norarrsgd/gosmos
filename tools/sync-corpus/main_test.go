package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncRejectsWrongSchemaWithoutWriting(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	if e := os.WriteFile(filepath.Join(src, "case.json"), []byte(`{"schemaVersion":2}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e := syncCorpus(src, dst); e == nil {
		t.Fatal("accepted incompatible schema")
	}
	if _, e := os.Stat(dst); !os.IsNotExist(e) {
		t.Fatal("wrote failed corpus")
	}
}
