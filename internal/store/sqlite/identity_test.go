package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"memgraphai/internal/domain"
	"memgraphai/internal/testkit"
)

func TestStorePreservesProjectIDsAndCase(t *testing.T) {
	store := openIdentityStore(t)
	if err := store.CreateProject(t.Context(), "project-a", "Roadmap"); err != nil {
		t.Fatalf("CreateProject(project-a) error = %v", err)
	}
	if err := store.CreateProject(t.Context(), "project-b", "Roadmap"); err != nil {
		t.Fatalf("CreateProject(project-b) error = %v", err)
	}
	if err := store.RenameProjectID(t.Context(), "project-a", "project-renamed"); !domain.IsOutcome(err, domain.Immutable) {
		t.Fatalf("RenameProjectID() error = %v, want %q", err, domain.Immutable)
	}
	if got, err := store.ProjectName(t.Context(), "project-a"); err != nil || got != "Roadmap" {
		t.Fatalf("ProjectName(project-a) = %q, %v; want %q, nil", got, err, "Roadmap")
	}
	if got, err := store.ProjectName(t.Context(), "project-b"); err != nil || got != "Roadmap" {
		t.Fatalf("ProjectName(project-b) = %q, %v; want %q, nil", got, err, "Roadmap")
	}
}

func TestResolvePathReportsOneManyNoneAndUnavailableWithoutGuessing(t *testing.T) {
	store := openIdentityStore(t)
	root := testkit.TempProjectDir(t, "project")
	alias := filepath.Join(t.TempDir(), "alias")
	testkit.SymlinkOrSkip(t, root, alias)
	for _, project := range []string{"project-a", "project-b"} {
		if err := store.CreateProject(t.Context(), project, project); err != nil {
			t.Fatalf("CreateProject(%q) error = %v", project, err)
		}
	}
	if err := store.AssociatePath(t.Context(), "project-a", root); err != nil {
		t.Fatalf("AssociatePath(root) error = %v", err)
	}
	if got, err := store.ResolvePath(t.Context(), alias); err != nil || got.Outcome != domain.One || len(got.Projects) != 1 || got.Projects[0] != "project-a" {
		t.Fatalf("ResolvePath(alias) = %#v, %v; want one project-a", got, err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("Mkdir(nested) error = %v", err)
	}
	if got, err := store.ResolvePath(t.Context(), nested); err != nil || got.Outcome != domain.None {
		t.Fatalf("ResolvePath(nested) = %#v, %v; want none without a prefix guess", got, err)
	}
	if err := store.AssociatePath(t.Context(), "project-b", alias); err != nil {
		t.Fatalf("AssociatePath(alias) error = %v", err)
	}
	if got, err := store.ResolvePath(t.Context(), root); err != nil || got.Outcome != domain.Many || len(got.Projects) != 2 {
		t.Fatalf("ResolvePath(root) = %#v, %v; want many projects", got, err)
	}
	moved := root + "-moved"
	if err := testkit.Move(root, moved); err != nil {
		t.Fatalf("Move(root) error = %v", err)
	}
	if got, err := store.ResolvePath(t.Context(), root); err != nil || got.Outcome != domain.Unavailable {
		t.Fatalf("ResolvePath(moved source) = %#v, %v; want unavailable", got, err)
	}
	if got, err := store.ResolvePath(t.Context(), moved); err != nil || got.Outcome != domain.None {
		t.Fatalf("ResolvePath(moved destination) = %#v, %v; want none", got, err)
	}
}

func TestStoreRejectsCrossProjectAndMismatchedBindings(t *testing.T) {
	store := openIdentityStore(t)
	for _, project := range []string{"project-a", "project-b"} {
		if err := store.CreateProject(t.Context(), project, project); err != nil {
			t.Fatalf("CreateProject(%q) error = %v", project, err)
		}
	}
	if err := store.CreateWorkstream(t.Context(), "stream-a", "project-a"); err != nil {
		t.Fatalf("CreateWorkstream() error = %v", err)
	}
	if err := store.CreateSession(t.Context(), "session-a", "project-b", "stream-a"); !domain.IsOutcome(err, domain.BindingMismatch) {
		t.Fatalf("CreateSession(cross-project) error = %v, want %q", err, domain.BindingMismatch)
	}
	if err := store.CreateSession(t.Context(), "session-a", "project-a", "stream-a"); err != nil {
		t.Fatalf("CreateSession(valid) error = %v", err)
	}
	if err := store.ValidateBinding(t.Context(), "project-b", "stream-a", "session-a"); !domain.IsOutcome(err, domain.BindingMismatch) {
		t.Fatalf("ValidateBinding(mismatch) error = %v, want %q", err, domain.BindingMismatch)
	}
	if err := store.ValidateBinding(t.Context(), "project-a", "stream-a", "session-a"); err != nil {
		t.Fatalf("ValidateBinding(valid) error = %v", err)
	}
}

func TestStoreRejectsForeignKeyViolationsOnEveryUsableConnection(t *testing.T) {
	store := openIdentityStore(t)
	first, err := store.db.Conn(t.Context())
	if err != nil {
		t.Fatalf("first database connection: %v", err)
	}
	defer first.Close()
	second, err := store.db.Conn(t.Context())
	if err != nil {
		t.Fatalf("second database connection: %v", err)
	}
	defer second.Close()

	if _, err := second.ExecContext(t.Context(), `INSERT INTO project_paths(project_id, supplied_path, resolved_path)
		VALUES ('missing-project', '/supplied', '/resolved')`); err == nil {
		t.Fatal("direct SQL foreign-key violation error = nil, want constraint rejection")
	}
}

func openIdentityStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.Context(), testkit.TempSQLitePath(t, "identity"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
