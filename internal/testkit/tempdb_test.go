package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTempSQLitePathBuildsAPathInsideTheTestDirectory(t *testing.T) {
	path := TempSQLitePath(t, "probe")

	if got, want := filepath.Base(path), "probe.sqlite"; got != want {
		t.Fatalf("TempSQLitePath() base = %q, want %q", got, want)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("TempSQLitePath() directory is unavailable: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("TempSQLitePath() parent is not a directory: %q", filepath.Dir(path))
	}
}

func TestTempSQLitePathSeparatesNamedArtifacts(t *testing.T) {
	first := TempSQLitePath(t, "first")
	second := TempSQLitePath(t, "second")

	if got, want := filepath.Base(first), "first.sqlite"; got != want {
		t.Fatalf("first base = %q, want %q", got, want)
	}
	if got, want := filepath.Base(second), "second.sqlite"; got != want {
		t.Fatalf("second base = %q, want %q", got, want)
	}
	if first == second {
		t.Fatalf("TempSQLitePath() returned one artifact path %q for distinct names", first)
	}
}
