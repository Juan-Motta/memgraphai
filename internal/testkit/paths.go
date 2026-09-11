package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

// TempProjectDir creates a project path isolated to a test.
func TempProjectDir(t testing.TB, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("create temporary project directory: %v", err)
	}
	return path
}

// SymlinkOrSkip creates a symlink or skips where the host denies it.
func SymlinkOrSkip(t testing.TB, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// Move renames a fixture path.
func Move(from, to string) error { return os.Rename(from, to) }
