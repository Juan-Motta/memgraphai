package domain

import "testing"

func TestAuthorizeProjectDoesNotTreatAssociationAsPermission(t *testing.T) {
	project := ProjectID("project-a")

	if err := AuthorizeProject(project, map[ProjectID]bool{"project-b": true}); !IsOutcome(err, ScopeDenied) {
		t.Fatalf("AuthorizeProject() error = %v, want %q", err, ScopeDenied)
	}
	if err := AuthorizeProject(project, map[ProjectID]bool{project: true}); err != nil {
		t.Fatalf("AuthorizeProject() with explicit grant error = %v", err)
	}
}
