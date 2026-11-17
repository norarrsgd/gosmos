//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func secureDirectory(path string) (string, os.FileInfo, error) {
	if e := validateConfiguredDirectory(path); e != nil {
		return "", nil, e
	}
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil {
		return "", nil, e
	}
	if e = validateCanonicalDirectory(resolved); e != nil {
		return "", nil, e
	}
	info, e := os.Stat(resolved)
	if e != nil {
		return "", nil, e
	}
	uid, ok := fileUID(info)
	if !info.IsDir() || info.Mode().Perm()&0022 != 0 || !ok || uid != uint32(os.Geteuid()) {
		return "", nil, fmt.Errorf("statistics directory must be owned by the current user and not group- or world-writable")
	}
	return resolved, info, nil
}

// validateConfiguredDirectory proves that every pathname entry used while resolving path
// is controlled by either the current user or root. A sticky shared directory is accepted
// only when its next entry is also owned by the current user or root.
func validateConfiguredDirectory(path string) error {
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(path, current), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		candidate := filepath.Join(current, part)
		parent, e := filepath.EvalSymlinks(filepath.Dir(candidate))
		if e != nil {
			return e
		}
		if e = validateCanonicalDirectory(parent); e != nil {
			return e
		}
		parentInfo, e := os.Stat(parent)
		if e != nil {
			return e
		}
		candidateInfo, e := os.Lstat(candidate)
		if e != nil {
			return e
		}
		if !protectedDirectoryEntry(parentInfo, candidateInfo) {
			return fmt.Errorf("statistics directory path contains an entry replaceable by another user")
		}
		if candidateInfo.Mode()&os.ModeSymlink != 0 {
			target, resolveErr := filepath.EvalSymlinks(candidate)
			if resolveErr != nil {
				return resolveErr
			}
			if e = validateCanonicalDirectory(target); e != nil {
				return e
			}
		} else if !candidateInfo.IsDir() {
			return fmt.Errorf("statistics directory path contains a non-directory entry")
		}
		current = candidate
	}
	return nil
}

func validateCanonicalDirectory(path string) error {
	current := string(filepath.Separator)
	parentInfo, e := os.Stat(current)
	if e != nil {
		return e
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, current), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		candidate := filepath.Join(current, part)
		candidateInfo, statErr := os.Lstat(candidate)
		if statErr != nil {
			return statErr
		}
		if candidateInfo.Mode()&os.ModeSymlink != 0 || !candidateInfo.IsDir() {
			return fmt.Errorf("resolved statistics directory path is not a directory")
		}
		if !protectedDirectoryEntry(parentInfo, candidateInfo) {
			return fmt.Errorf("statistics directory path contains an entry replaceable by another user")
		}
		current = candidate
		parentInfo = candidateInfo
	}
	return nil
}

func protectedDirectoryEntry(parent, entry os.FileInfo) bool {
	parentUID, parentOK := fileUID(parent)
	entryUID, entryOK := fileUID(entry)
	effectiveUID := uint32(os.Geteuid())
	if !parentOK || !entryOK || parentUID != 0 && parentUID != effectiveUID {
		return false
	}
	if parent.Mode().Perm()&0022 == 0 {
		return true
	}
	return parent.Mode()&os.ModeSticky != 0 && (entryUID == 0 || entryUID == effectiveUID)
}

func ownedByProcess(info os.FileInfo) bool {
	uid, ok := fileUID(info)
	return ok && uid == uint32(os.Geteuid())
}

func openFileNoFollow(path string, flag int, perm os.FileMode) (*os.File, error) {
	fd, e := syscall.Open(path, flag|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, uint32(perm.Perm()))
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(fd), path), nil
}

func fileUID(info os.FileInfo) (uint32, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return stat.Uid, true
}
