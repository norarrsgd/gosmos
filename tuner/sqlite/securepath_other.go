//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
)

func secureDirectory(path string) (string, os.FileInfo, error) {
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil {
		return "", nil, e
	}
	info, e := os.Stat(resolved)
	if e != nil {
		return "", nil, e
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("statistics directory path is not a directory")
	}
	return resolved, info, nil
}

// Platforms without Unix ownership and mode bits rely on the retained directory and
// file handles plus the live database identity check in Open.
func ownedByProcess(os.FileInfo) bool { return true }

func openFileNoFollow(path string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag, perm)
}
