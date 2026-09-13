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

func TestPrepareAttemptRetriesAfterLinkWithoutClobbering(t *testing.T) {
	library := filepath.Join(t.TempDir(), "library")
	interrupted := New(library, func(point Point) error {
		if point == AfterPublish {
			return errors.New("interrupted after link")
		}
		return nil
	})
	if _, err := interrupted.PrepareAttempt("operation-1", 1, "project-1", "document-1", "revision-1", []byte("# first\n")); err == nil {
		t.Fatal("PrepareAttempt() error = nil, want post-link interruption")
	}
	prepared, err := New(library, nil).PrepareAttempt("operation-1", 1, "project-1", "document-1", "revision-1", []byte("# first\n"))
	if err != nil {
		t.Fatalf("PrepareAttempt(retry) error = %v", err)
	}
	content, err := os.ReadFile(prepared.Path)
	if err != nil || string(content) != "# first\n" {
		t.Fatalf("ReadFile(retried immutable revision) = %q, %v; want original", content, err)
	}
	if _, err := New(library, nil).PrepareAttempt("operation-1", 2, "project-1", "document-1", "revision-1", []byte("# changed\n")); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("PrepareAttempt(different payload) error = %v, want integrity discrepancy", err)
	}
}

func TestPrepareAttemptRejectsMatchingFinalLeafSymlink(t *testing.T) {
	library := filepath.Join(t.TempDir(), "library")
	finalDir := filepath.Join(library, "projects", "project-1", "documents", "document-1", "revisions")
	if err := os.MkdirAll(finalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(final directory) error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	markdown := []byte("# matching\n")
	if err := os.WriteFile(outside, markdown, 0o600); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	finalPath := filepath.Join(finalDir, "revision-1.md")
	testkit.SymlinkOrSkip(t, outside, finalPath)

	if _, err := New(library, nil).PrepareAttempt("operation-1", 1, "project-1", "document-1", "revision-1", markdown); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("PrepareAttempt(matching final symlink) error = %v, want integrity discrepancy", err)
	}
	if err := os.WriteFile(outside, []byte("# externally changed\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(external mutation) error = %v", err)
	}
	content, err := os.ReadFile(finalPath)
	if err != nil || string(content) != "# externally changed\n" {
		t.Fatalf("ReadFile(final symlink after external mutation) = %q, %v; want changed target", content, err)
	}
}

func TestPrepareAttemptRejectsMatchingStagingLeafSymlink(t *testing.T) {
	library := filepath.Join(t.TempDir(), "library")
	stagingDir := filepath.Join(library, ".staging", "operation-1", "1")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(staging directory) error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.tmp")
	markdown := []byte("# matching\n")
	if err := os.WriteFile(outside, markdown, 0o600); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	testkit.SymlinkOrSkip(t, outside, filepath.Join(stagingDir, "revision-1.tmp"))

	if _, err := New(library, nil).PrepareAttempt("operation-1", 1, "project-1", "document-1", "revision-1", markdown); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("PrepareAttempt(matching staging symlink) error = %v, want integrity discrepancy", err)
	}
}

func TestPrepareAttemptRejectsNonRegularLeaves(t *testing.T) {
	for _, leaf := range []string{"staging", "final"} {
		t.Run(leaf, func(t *testing.T) {
			library := filepath.Join(t.TempDir(), "library")
			var path string
			if leaf == "staging" {
				path = filepath.Join(library, ".staging", "operation-1", "1", "revision-1.tmp")
			} else {
				path = filepath.Join(library, "projects", "project-1", "documents", "document-1", "revisions", "revision-1.md")
			}
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("MkdirAll(%s leaf) error = %v", leaf, err)
			}

			if _, err := New(library, nil).PrepareAttempt("operation-1", 1, "project-1", "document-1", "revision-1", []byte("# text\n")); !errors.Is(err, ErrIntegrity) {
				t.Fatalf("PrepareAttempt(%s directory leaf) error = %v, want integrity discrepancy", leaf, err)
			}
		})
	}
}

func TestVerifyRejectsSymlinkLeaf(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.md")
	content := []byte("# matching\n")
	if err := os.WriteFile(outside, content, 0o600); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	linked := filepath.Join(t.TempDir(), "revision.md")
	testkit.SymlinkOrSkip(t, outside, linked)
	digest := sha256.Sum256(content)

	if err := Verify(linked, hex.EncodeToString(digest[:]), int64(len(content))); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Verify(matching symlink) error = %v, want integrity discrepancy", err)
	}
}

func TestPrepareRejectsSymlinkedRootAndIntermediateBoundaries(t *testing.T) {
	realRoot := filepath.Join(t.TempDir(), "real-library")
	linkedRoot := filepath.Join(t.TempDir(), "linked-library")
	testkit.SymlinkOrSkip(t, realRoot, linkedRoot)
	if _, err := New(linkedRoot, nil).Prepare("operation-1", "project-1", "document-1", "revision-1", []byte("# text\n")); !errors.Is(err, ErrSymlinkBoundary) {
		t.Fatalf("Prepare(symlink root) error = %v, want symlink boundary", err)
	}
	library := filepath.Join(t.TempDir(), "library")
	if err := os.MkdirAll(filepath.Join(library, "projects"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatalf("Mkdir(outside) error = %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(library, "projects", "project-1")); err != nil {
		t.Fatalf("Symlink(intermediate) error = %v", err)
	}
	if _, err := New(library, nil).Prepare("operation-1", "project-1", "document-1", "revision-1", []byte("# text\n")); !errors.Is(err, ErrSymlinkBoundary) {
		t.Fatalf("Prepare(symlink intermediate) error = %v, want symlink boundary", err)
	}
}

func TestPrepareRejectsPrepopulatedDescendantsBehindIntermediateSymlink(t *testing.T) {
	for _, boundary := range []string{"revision", "staging"} {
		t.Run(boundary, func(t *testing.T) {
			library := filepath.Join(t.TempDir(), "library")
			outside := filepath.Join(t.TempDir(), "outside")
			if boundary == "revision" {
				if err := os.MkdirAll(filepath.Join(outside, "documents", "document-1", "revisions"), 0o755); err != nil {
					t.Fatalf("MkdirAll(outside revision) error = %v", err)
				}
				if err := os.MkdirAll(filepath.Join(library, "projects"), 0o755); err != nil {
					t.Fatalf("MkdirAll(projects) error = %v", err)
				}
				testkit.SymlinkOrSkip(t, outside, filepath.Join(library, "projects", "project-1"))
			} else {
				if err := os.MkdirAll(filepath.Join(library, "projects", "project-1", "documents", "document-1", "revisions"), 0o755); err != nil {
					t.Fatalf("MkdirAll(revisions) error = %v", err)
				}
				if err := os.MkdirAll(filepath.Join(outside, "operation-1"), 0o755); err != nil {
					t.Fatalf("MkdirAll(outside staging) error = %v", err)
				}
				testkit.SymlinkOrSkip(t, outside, filepath.Join(library, ".staging"))
			}
			if _, err := New(library, nil).Prepare("operation-1", "project-1", "document-1", "revision-1", []byte("# text\n")); !errors.Is(err, ErrSymlinkBoundary) {
				t.Fatalf("Prepare() error = %v, want symlink boundary", err)
			}
		})
	}
}

func TestPrepareMissingRootChecksNearestEstablishedAncestor(t *testing.T) {
	parent := t.TempDir()
	outside := t.TempDir()
	linkedAncestor := filepath.Join(parent, "linked")
	testkit.SymlinkOrSkip(t, outside, linkedAncestor)
	library := filepath.Join(linkedAncestor, "missing", "library")

	if _, err := New(library, nil).Prepare("operation-1", "project-1", "document-1", "revision-1", []byte("# text\n")); !errors.Is(err, ErrSymlinkBoundary) {
		t.Fatalf("Prepare(missing root below symlink) error = %v, want symlink boundary", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(outside missing directory) error = %v, want not exist", err)
	}
}

func TestPrepareCreatesMissingRootBelowRealEstablishedAncestor(t *testing.T) {
	anchor := filepath.Join(t.TempDir(), "anchor")
	if err := os.Mkdir(anchor, 0o755); err != nil {
		t.Fatalf("Mkdir(anchor) error = %v", err)
	}
	library := filepath.Join(anchor, "missing", "library")

	if _, err := New(library, nil).Prepare("operation-1", "project-1", "document-1", "revision-1", []byte("# text\n")); err != nil {
		t.Fatalf("Prepare(missing root below real ancestor) error = %v", err)
	}
	if info, err := os.Lstat(library); err != nil || !info.IsDir() {
		t.Fatalf("Lstat(created root) = %v, %v; want directory", info, err)
	}
}

func TestPrepareAllowsEstablishedAnchorReachedThroughSymlinkAncestor(t *testing.T) {
	parent := t.TempDir()
	realParent := t.TempDir()
	anchor := filepath.Join(realParent, "anchor")
	if err := os.Mkdir(anchor, 0o755); err != nil {
		t.Fatalf("Mkdir(anchor) error = %v", err)
	}
	alias := filepath.Join(parent, "alias")
	testkit.SymlinkOrSkip(t, realParent, alias)
	library := filepath.Join(alias, "anchor", "missing-library")

	if _, err := New(library, nil).Prepare("operation-1", "project-1", "document-1", "revision-1", []byte("# text\n")); err != nil {
		t.Fatalf("Prepare(missing root below established aliased anchor) error = %v", err)
	}
	if info, err := os.Lstat(filepath.Join(anchor, "missing-library")); err != nil || !info.IsDir() {
		t.Fatalf("Lstat(created root through alias) = %v, %v; want directory", info, err)
	}
}

func TestEnsureDirAcceptsOnlyDirectoryRaceWinner(t *testing.T) {
	for _, winner := range []string{"directory", "symlink", "file"} {
		t.Run(winner, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "winner")
			mkdir := func(name string, perm os.FileMode) error {
				var err error
				switch winner {
				case "directory":
					err = os.Mkdir(name, perm)
				case "symlink":
					outside := t.TempDir()
					err = os.Symlink(outside, name)
				case "file":
					err = os.WriteFile(name, []byte("not a directory"), 0o600)
				}
				if err != nil {
					t.Fatalf("create %s race winner: %v", winner, err)
				}
				return &os.PathError{Op: "mkdir", Path: name, Err: os.ErrExist}
			}

			err := ensureDirWithMkdir(root, path, mkdir)
			if winner == "directory" && err != nil {
				t.Fatalf("ensureDirWithMkdir(directory winner) error = %v, want nil", err)
			}
			if winner != "directory" && err == nil {
				t.Fatalf("ensureDirWithMkdir(%s winner) error = nil, want rejection", winner)
			}
			if winner == "symlink" && !errors.Is(err, ErrSymlinkBoundary) {
				t.Fatalf("ensureDirWithMkdir(symlink winner) error = %v, want symlink boundary", err)
			}
		})
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
