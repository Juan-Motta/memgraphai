package revisionfs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"memgraphai/internal/testkit"
)

func TestPreparePublishesImmutableMarkdown(t *testing.T) {
	library := filepath.Join(t.TempDir(), "library")
	fs := New(library, nil)

	prepared, err := fs.Prepare("operation-1", "project-1", "document-1", "revision-1", []byte("# first\n"))
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	want := filepath.Join(library, "projects", "project-1", "documents", "document-1", "revisions", "revision-1.md")
	if prepared.Path != want {
		t.Fatalf("Prepare().Path = %q, want %q", prepared.Path, want)
	}
	if prepared.Bytes != int64(len("# first\n")) {
		t.Fatalf("Prepare().Bytes = %d, want %d", prepared.Bytes, len("# first\n"))
	}
	digest := sha256.Sum256([]byte("# first\n"))
	if prepared.Checksum != hex.EncodeToString(digest[:]) {
		t.Fatalf("Prepare().Checksum = %q, want SHA-256 of Markdown", prepared.Checksum)
	}
	content, err := os.ReadFile(prepared.Path)
	if err != nil {
		t.Fatalf("ReadFile(prepared revision) error = %v", err)
	}
	if got := string(content); got != "# first\n" {
		t.Fatalf("prepared content = %q, want %q", got, "# first\n")
	}
}

func TestPrepareDoesNotClobberAnExistingRevisionOrAcceptUnsafeIDs(t *testing.T) {
	library := filepath.Join(t.TempDir(), "library")
	fs := New(library, nil)
	first, err := fs.Prepare("operation-1", "project-1", "document-1", "revision-1", []byte("# first\n"))
	if err != nil {
		t.Fatalf("Prepare(first) error = %v", err)
	}
	if _, err := fs.Prepare("operation-2", "project-1", "document-1", "revision-1", []byte("# replacement\n")); !errors.Is(err, ErrAlreadyPublished) {
		t.Fatalf("Prepare(duplicate) error = %v, want ErrAlreadyPublished", err)
	}
	content, err := os.ReadFile(first.Path)
	if err != nil || string(content) != "# first\n" {
		t.Fatalf("ReadFile(first) = %q, %v; want original immutable content", content, err)
	}
	if _, err := fs.Prepare("operation-3", "project-1", "document-1", "../escape", []byte("# unsafe\n")); !errors.Is(err, ErrUnsafeID) {
		t.Fatalf("Prepare(unsafe ID) error = %v, want ErrUnsafeID", err)
	}
	if _, err := os.Stat(filepath.Join(library, "projects", "project-1", "documents", "document-1", "escape.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe revision path stat error = %v, want not exist", err)
	}
}

func TestPrepareInterruptionBeforePublishLeavesNoRevision(t *testing.T) {
	library := filepath.Join(t.TempDir(), "library")
	injected := errors.New("injected interruption")
	fault := testkit.FailAt(string(BeforePublish), injected)
	fs := New(library, func(point Point) error { return fault.At(string(point)) })

	if _, err := fs.Prepare("operation-1", "project-1", "document-1", "revision-1", []byte("# interrupted\n")); !errors.Is(err, injected) {
		t.Fatalf("Prepare(interrupted) error = %v, want injected interruption", err)
	}
	finalPath := filepath.Join(library, "projects", "project-1", "documents", "document-1", "revisions", "revision-1.md")
	if _, err := os.Stat(finalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(unpublished revision) error = %v, want not exist", err)
	}
}
