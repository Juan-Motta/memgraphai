package testkit

import (
	"path/filepath"
	"testing"
)

// TempSQLitePath returns an isolated SQLite database path for a test.
func TempSQLitePath(t testing.TB, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name+".sqlite")
}
