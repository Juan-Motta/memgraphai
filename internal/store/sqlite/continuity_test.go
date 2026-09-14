package sqlite

import (
	"errors"
	"testing"
	"time"

	"memgraphai/internal/domain"
	"memgraphai/internal/testkit"
)

func TestContinuityStoreUsesCASLifecycleAndDatabaseOwnershipConstraints(t *testing.T) {
	store, err := Open(t.Context(), testkit.TempSQLitePath(t, "continuity-store"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, projectID := range []string{"project-a", "project-b"} {
		if err := store.CreateProject(t.Context(), projectID, projectID); err != nil {
			t.Fatalf("CreateProject(%q) error = %v", projectID, err)
		}
	}
	at := time.Date(2026, 3, 2, 3, 4, 5, 0, time.UTC)
	if err := store.CreateContinuityWorkstream(t.Context(), "project-a", "stream-a", "cli", "terminal", "reported-model", at); err != nil {
		t.Fatalf("CreateContinuityWorkstream() error = %v", err)
	}
	if err := store.OpenSession(t.Context(), "project-a", "stream-a", "session-a", "cli", "terminal", "reported-model", at); err != nil {
		t.Fatalf("OpenSession() error = %v", err)
	}
	if err := store.CreateContinuityWorkstream(t.Context(), "project-b", "stream-b", "cli", "", "", at); err != nil {
		t.Fatalf("CreateContinuityWorkstream(project-b) error = %v", err)
	}
	if err := store.DisconnectSession(t.Context(), "project-a", "stream-a", "session-a", "adapter-observation", at); err != nil {
		t.Fatalf("DisconnectSession() error = %v", err)
	}
	if err := store.ValidateActiveBinding(t.Context(), "project-a", "stream-a", "session-a"); !continuityHasCode(err, continuityConflict) {
		t.Fatalf("ValidateActiveBinding(disconnected) error = %v, want conflict", err)
	}
	if err := store.CloseSession(t.Context(), "project-a", "stream-a", "session-a", at); err != nil {
		t.Fatalf("CloseSession(disconnected) error = %v", err)
	}
	if err := store.ValidateActiveBinding(t.Context(), "project-a", "stream-a", "session-a"); !continuityHasCode(err, continuityConflict) {
		t.Fatalf("ValidateActiveBinding(closed) error = %v, want conflict", err)
	}
	if err := store.CloseSession(t.Context(), "project-a", "stream-a", "session-a", at); !continuityHasCode(err, continuityConflict) {
		t.Fatalf("second CloseSession() error = %v, want CAS conflict", err)
	}
	status, _, _, _, _, _, err := store.SessionStatus(t.Context(), "project-a", "stream-a", "session-a")
	if err != nil || status != "closed" {
		t.Fatalf("SessionStatus(closed) = %q, %v; want structural closed lookup", status, err)
	}

	connection, err := store.db.Conn(t.Context())
	if err != nil {
		t.Fatalf("second usable connection: %v", err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(t.Context(), `INSERT INTO workstreams(
		workstream_id, project_id, forked_from_workstream_id, origin
	) VALUES ('stream-cross', 'project-b', 'stream-a', 'direct')`); err == nil {
		t.Fatal("cross-project direct fork error = nil, want database rejection")
	}
	if err := store.ForkWorkstream(t.Context(), "project-a", "stream-a", "stream-child", "cli", "", "", at); err != nil {
		t.Fatalf("ForkWorkstream() error = %v", err)
	}
	if _, err := connection.ExecContext(t.Context(), `UPDATE workstreams
		SET forked_from_workstream_id = 'stream-b' WHERE workstream_id = 'stream-child'`); err == nil {
		t.Fatal("cross-project direct fork update error = nil, want database rejection")
	}
	if _, err := connection.ExecContext(t.Context(), `INSERT INTO sessions(
		session_id, project_id, workstream_id, status, origin, resumed_from_session_id
	) VALUES ('session-cross', 'project-b', 'stream-b', 'open', 'direct', 'session-a')`); err == nil {
		t.Fatal("cross-project direct resume source error = nil, want database rejection")
	}
}

func TestContinuityStoreDistinguishesMissingAndMismatchedSessionTriples(t *testing.T) {
	store, err := Open(t.Context(), testkit.TempSQLitePath(t, "continuity-triples"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-a", "Alpha"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := store.CreateContinuityWorkstream(t.Context(), "project-a", "stream-a", "cli", "", "", time.Now()); err != nil {
		t.Fatalf("CreateContinuityWorkstream() error = %v", err)
	}
	if err := store.OpenSession(t.Context(), "project-a", "stream-a", "session-a", "cli", "", "", time.Now()); err != nil {
		t.Fatalf("OpenSession() error = %v", err)
	}
	_, _, _, _, _, _, err = store.SessionStatus(t.Context(), "project-a", "stream-a", "missing")
	if !continuityHasCode(err, continuityNotFound) {
		t.Fatalf("missing SessionStatus() error = %v, want not_found", err)
	}
	_, _, _, _, _, _, err = store.SessionStatus(t.Context(), "project-a", "stream-other", "session-a")
	if !domain.IsOutcome(err, domain.BindingMismatch) {
		t.Fatalf("mismatched SessionStatus() error = %v, want binding_mismatch", err)
	}
}

func continuityHasCode(err error, code string) bool {
	var continuity ContinuityError
	return errors.As(err, &continuity) && continuity.Code == code
}
