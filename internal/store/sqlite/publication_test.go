package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
)

func TestPublishRevisionMakesFirstPreparedRevisionCurrent(t *testing.T) {
	store, fs := newPublicationFixture(t)
	first := preparePublication(t, fs, "revision-1", "# first\n", nil)

	if err := store.PublishRevision(t.Context(), first); err != nil {
		t.Fatalf("PublishRevision(first) error = %v", err)
	}
	assertCurrentRevision(t, store, "revision-1")
}

func TestPublishRevisionReplacesCurrentOnlyWhenExpectedRevisionMatches(t *testing.T) {
	store, fs := newPublicationFixture(t)
	first := preparePublication(t, fs, "revision-1", "# first\n", nil)
	if err := store.PublishRevision(t.Context(), first); err != nil {
		t.Fatalf("PublishRevision(first) error = %v", err)
	}
	second := preparePublication(t, fs, "revision-2", "# second\n", stringPointer("revision-1"))
	if err := store.PublishRevision(t.Context(), second); err != nil {
		t.Fatalf("PublishRevision(replacement) error = %v", err)
	}
	stale := preparePublication(t, fs, "revision-3", "# stale\n", stringPointer("revision-1"))
	if err := store.PublishRevision(t.Context(), stale); !domain.IsOutcome(err, domain.Conflict) {
		t.Fatalf("PublishRevision(stale) error = %v, want %q", err, domain.Conflict)
	}
	assertCurrentRevision(t, store, "revision-2")
}

func TestPublishRevisionPreCommitInterruptionPreservesPriorCurrent(t *testing.T) {
	store, fs := newPublicationFixture(t)
	first := preparePublication(t, fs, "revision-1", "# first\n", nil)
	if err := store.PublishRevision(t.Context(), first); err != nil {
		t.Fatalf("PublishRevision(first) error = %v", err)
	}
	second := preparePublication(t, fs, "revision-2", "# interrupted\n", stringPointer("revision-1"))
	injected := errors.New("injected pre-commit interruption")
	store.beforeCommit = func() error { return injected }

	if err := store.PublishRevision(t.Context(), second); !errors.Is(err, injected) {
		t.Fatalf("PublishRevision(interrupted) error = %v, want injected interruption", err)
	}
	assertCurrentRevision(t, store, "revision-1")
	content, err := os.ReadFile(second.FilePath)
	if err != nil || string(content) != "# interrupted\n" {
		t.Fatalf("ReadFile(prepared but invisible revision) = %q, %v; want prepared Markdown", content, err)
	}
}

func TestPublishRevisionResponseLossIsUnknownWithoutRollback(t *testing.T) {
	store, fs := newPublicationFixture(t)
	first := preparePublication(t, fs, "revision-1", "# first\n", nil)
	store.afterCommit = func() error { return errors.New("injected lost response") }

	if err := store.PublishRevision(t.Context(), first); !domain.IsOutcome(err, domain.Unknown) {
		t.Fatalf("PublishRevision(response loss) error = %v, want %q", err, domain.Unknown)
	}
	assertCurrentRevision(t, store, "revision-1")
}

func newPublicationFixture(t *testing.T) (*Store, *revisionfs.Filesystem) {
	t.Helper()
	store := openIdentityStore(t)
	if err := store.CreateProject(t.Context(), "project-1", "Project"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := store.CreateDocument(t.Context(), "document-1", "project-1"); err != nil {
		t.Fatalf("CreateDocument() error = %v", err)
	}
	return store, revisionfs.New(filepath.Join(t.TempDir(), "library"), nil)
}

func preparePublication(t *testing.T, fs *revisionfs.Filesystem, revisionID, content string, expected *string) Publication {
	t.Helper()
	prepared, err := fs.Prepare("operation-"+revisionID, "project-1", "document-1", revisionID, []byte(content))
	if err != nil {
		t.Fatalf("Prepare(%s) error = %v", revisionID, err)
	}
	return Publication{
		DocumentID:      "document-1",
		RevisionID:      revisionID,
		ExpectedCurrent: expected,
		FilePath:        prepared.Path,
		Checksum:        prepared.Checksum,
		ByteCount:       prepared.Bytes,
	}
}

func assertCurrentRevision(t *testing.T, store *Store, want string) {
	t.Helper()
	got, exists, err := store.CurrentRevision(t.Context(), "document-1")
	if err != nil || !exists || got != want {
		t.Fatalf("CurrentRevision() = %q, %t, %v; want %q, true, nil", got, exists, err, want)
	}
}

func stringPointer(value string) *string { return &value }
