package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeCommandReportsSQLiteEvidence(t *testing.T) {
	command := exec.Command("go", "run", ".", "probe")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go run . probe error = %v\noutput:\n%s", err, output)
	}

	if got, want := strings.TrimSpace(string(output)), "memgraphai: SQLite FTS5 probe succeeded (matches=1, transaction=committed)"; got != want {
		t.Fatalf("go run . probe output = %q, want %q", got, want)
	}
}

func TestProjectCLIProcessUsesHumanAndJSONForAllOperations(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	library, path := t.TempDir(), filepath.Join(t.TempDir(), "project-path")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("create path fixture: %v", err)
	}
	run := func(input string) (map[string]any, error) {
		t.Helper()
		command := exec.Command(binary, "--library", library, "--json")
		command.Stdin = strings.NewReader(input)
		output, err := command.CombinedOutput()
		var response map[string]any
		if decodeErr := json.Unmarshal(output, &response); decodeErr != nil {
			t.Fatalf("decode CLI response %q: %v", output, decodeErr)
		}
		return response, err
	}
	request := func(id, operation string, scope, input map[string]any) string {
		encoded, _ := json.Marshal(map[string]any{"contract_version": "memgraphai.experimental/v1alpha1", "operation_id": id, "operation": operation, "scope": scope, "input": input})
		return string(encoded)
	}
	assert := func(want, id, operation string, scope, input map[string]any) map[string]any {
		response, err := run(request(id, operation, scope, input))
		code := response["outcome"].(map[string]any)["code"]
		if (want == "ok") != (err == nil) || code != want {
			t.Fatalf("CLI %s = %v/%#v, want %s", operation, err, response, want)
		}
		return response
	}
	human := exec.Command(binary, "--library", library, "--operation-id", "human-create", "project", "create", "--id", "project-a", "--name", "Alpha")
	if output, err := human.CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "ok" {
		t.Fatalf("human create = %v/%q, want ok", err, output)
	}
	libraryScope := map[string]any{"kind": "library"}
	projectA := map[string]any{"kind": "project", "project_id": "project-a"}
	projectB := map[string]any{"kind": "project", "project_id": "project-b"}
	assert("ok", "create-b", "project.create", libraryScope, map[string]any{"project_id": "project-b", "name": "Beta"})
	assert("ok", "list", "project.list", libraryScope, map[string]any{})
	paged, _ := json.Marshal(map[string]any{"contract_version": "memgraphai.experimental/v1alpha1", "operation_id": "page", "operation": "project.list", "scope": libraryScope, "input": map[string]any{}, "page": map[string]any{"limit": 1}})
	if response, err := run(string(paged)); err != nil || response["page"] == nil {
		t.Fatalf("CLI paged list = %v/%#v, want continuation page", err, response)
	}
	assert("ok", "add-a", "project.association.add", projectA, map[string]any{"path": path})
	secondPath := filepath.Join(t.TempDir(), "project-path-2")
	if err := os.Mkdir(secondPath, 0o755); err != nil {
		t.Fatalf("create second path fixture: %v", err)
	}
	assert("ok", "add-a-2", "project.association.add", projectA, map[string]any{"path": secondPath})
	associationPage, _ := json.Marshal(map[string]any{"contract_version": "memgraphai.experimental/v1alpha1", "operation_id": "associations-page", "operation": "project.association.list", "scope": projectA, "input": map[string]any{}, "page": map[string]any{"limit": 1}})
	firstAssociations, err := run(string(associationPage))
	if err != nil || firstAssociations["page"] == nil {
		t.Fatalf("CLI association page = %v/%#v, want continuation", err, firstAssociations)
	}
	nextToken := firstAssociations["page"].(map[string]any)["next_token"]
	associationNext, _ := json.Marshal(map[string]any{"contract_version": "memgraphai.experimental/v1alpha1", "operation_id": "associations-next", "operation": "project.association.list", "scope": projectA, "input": map[string]any{}, "page": map[string]any{"limit": 1, "token": nextToken}})
	if continued, err := run(string(associationNext)); err != nil || continued["outcome"].(map[string]any)["code"] != "ok" {
		t.Fatalf("CLI association continuation = %v/%#v, want ok", err, continued)
	}
	alias := filepath.Join(t.TempDir(), "project-alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatalf("create alias fixture: %v", err)
	}
	assert("ok", "alias", "project.resolve", libraryScope, map[string]any{"path": alias})
	nested := filepath.Join(path, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("create nested fixture: %v", err)
	}
	assert("not_found", "nested", "project.resolve", libraryScope, map[string]any{"path": nested})
	assert("ok", "add-b", "project.association.add", projectB, map[string]any{"path": path})
	assert("ambiguous", "ambiguous", "project.resolve", libraryScope, map[string]any{"path": path})
	assert("scope_denied", "missing-scope", "project.association.list", libraryScope, map[string]any{})
	moved := path + "-moved"
	if err := os.Rename(path, moved); err != nil {
		t.Fatalf("move path fixture: %v", err)
	}
	assert("not_found", "unavailable", "project.resolve", libraryScope, map[string]any{"path": path})
	assert("ok", "remove", "project.association.remove", projectA, map[string]any{"path": path})
}

func TestCommandRejectsAnUnknownArgument(t *testing.T) {
	command := exec.Command("go", "run", ".", "unknown")
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("go run . unknown succeeded with output %q", output)
	}

	if got, want := strings.SplitN(strings.TrimSpace(string(output)), "\n", 2)[0], "usage: memgraphai probe"; got != want {
		t.Fatalf("go run . unknown application output = %q, want %q", got, want)
	}
}

func TestContinuityCLIProcessUsesHumanAndJSON(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	library := t.TempDir()
	runJSON := func(id, operation string, scope, input map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{
			"contract_version": "memgraphai.experimental/v1alpha1", "operation_id": id,
			"operation": operation, "scope": scope, "input": input,
		})
		command := exec.Command(binary, "--library", library, "--json")
		command.Stdin = strings.NewReader(string(raw))
		output, err := command.CombinedOutput()
		var response map[string]any
		if decodeErr := json.Unmarshal(output, &response); decodeErr != nil {
			t.Fatalf("decode %s response %q: %v", operation, output, decodeErr)
		}
		code := response["outcome"].(map[string]any)["code"].(string)
		if (code == "ok") != (err == nil) {
			t.Fatalf("%s = %v/%#v", operation, err, response)
		}
		return response
	}
	libraryScope := map[string]any{"kind": "library"}
	projectScope := map[string]any{"kind": "project", "project_id": "project-a"}
	workstreamScope := map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "workstream-a"}
	sessionScope := map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-a", "session_id": "session-a"}
	if code := runJSON("continuity-project", "project.create", libraryScope, map[string]any{"project_id": "project-a", "name": "Alpha"})["outcome"].(map[string]any)["code"]; code != "ok" {
		t.Fatalf("project setup = %q, want ok", code)
	}
	human := exec.Command(binary, "--library", library, "--operation-id", "continuity-human-workstream", "workstream", "create", "--project-id", "project-a", "--workstream-id", "workstream-human", "--origin", "shell")
	if output, err := human.CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "ok" {
		t.Fatalf("human workstream create = %v/%q, want ok", err, output)
	}
	assert := func(want, id, operation string, scope, input map[string]any) map[string]any {
		response := runJSON(id, operation, scope, input)
		if code := response["outcome"].(map[string]any)["code"]; code != want {
			t.Fatalf("%s outcome = %q, want %q", operation, code, want)
		}
		return response
	}
	assert("ok", "continuity-create", "workstream.create", projectScope, map[string]any{"workstream_id": "workstream-a", "origin": "cli"})
	listed := assert("ok", "continuity-list", "workstream.list", projectScope, map[string]any{})
	if len(listed["result"].(map[string]any)["workstreams"].([]any)) != 2 {
		t.Fatalf("workstream list did not return both explicit workstreams: %#v", listed)
	}
	assert("ok", "continuity-fork", "workstream.fork", workstreamScope, map[string]any{"workstream_id": "workstream-fork", "origin": "cli"})
	assert("ok", "continuity-open", "session.open", workstreamScope, map[string]any{"session_id": "session-a", "origin": "cli"})
	assert("ok", "continuity-status-open", "session.status", sessionScope, map[string]any{})
	assert("ok", "continuity-disconnect", "session.disconnect", sessionScope, map[string]any{"observed_by": "cli-client"})
	assert("ok", "continuity-status-disconnected", "session.status", sessionScope, map[string]any{})
	assert("ok", "continuity-close", "session.close", sessionScope, map[string]any{})
	closed := assert("ok", "continuity-status-closed", "session.status", sessionScope, map[string]any{})
	if status := closed["result"].(map[string]any)["status"]; status != "closed" {
		t.Fatalf("closed status = %q, want closed", status)
	}
	resumed := assert("ok", "continuity-resume", "session.resume", workstreamScope, map[string]any{"session_id": "session-resumed", "source_session_id": "session-a", "origin": "cli"})
	if source := resumed["result"].(map[string]any)["resumed_from_session_id"]; source != "session-a" {
		t.Fatalf("resume source = %q, want session-a", source)
	}
	wrongScope := map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-fork", "session_id": "session-a"}
	assert("binding_mismatch", "continuity-wrong-scope", "session.status", wrongScope, map[string]any{})
}
