package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
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

func TestDocumentCLIProcessUsesAllHumanAndJSONOperations(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	library := t.TempDir()
	runJSON := func(id, operation string, scope, input map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"contract_version": "memgraphai.experimental/v1alpha1", "operation_id": id, "operation": operation, "scope": scope, "input": input})
		command := exec.Command(binary, "--library", library, "--json")
		command.Stdin = strings.NewReader(string(raw))
		output, err := command.CombinedOutput()
		var response map[string]any
		if decodeErr := json.Unmarshal(output, &response); decodeErr != nil {
			t.Fatalf("decode %s response %q: %v", operation, output, decodeErr)
		}
		if code := response["outcome"].(map[string]any)["code"]; (code == "ok") != (err == nil) {
			t.Fatalf("JSON %s = %v/%#v", operation, err, response)
		}
		return response
	}
	project := map[string]any{"kind": "project", "project_id": "project-a"}
	runJSON("documents-project", "project.create", map[string]any{"kind": "library"}, map[string]any{"project_id": "project-a", "name": "Alpha"})
	content := "AP9NYXJrZG93bgo"
	for _, commandArgs := range [][]string{
		{"--operation-id", "human-create", "document", "create", "--project-id", "project-a", "--document-id", "human-doc", "--revision-id", "human-r1", "--expected-revision-id", "null", "--content-base64", content, "--provenance-origin", "shell", "--provenance-client", "terminal", "--provenance-model", "model-a"},
		{"--operation-id", "human-update", "document", "update", "--project-id", "project-a", "--document-id", "human-doc", "--revision-id", "human-r2", "--expected-revision-id", "human-r1", "--content-base64", content, "--provenance-origin", "shell"},
		{"--operation-id", "human-list", "document", "list", "--project-id", "project-a", "--limit", "1"},
		{"--operation-id", "human-read", "document", "read", "--project-id", "project-a", "--document-id", "human-doc"},
		{"--operation-id", "human-history", "document", "history", "--project-id", "project-a", "--document-id", "human-doc", "--limit", "1"},
	} {
		output, err := exec.Command(binary, append([]string{"--library", library}, commandArgs...)...).CombinedOutput()
		if err != nil || string(output) != "ok\n" {
			t.Fatalf("human document command %q = %v/%q, want exact ok newline", commandArgs, err, output)
		}
	}
	input := map[string]any{"document_id": "json-doc", "revision_id": "json-r1", "expected_revision_id": nil, "content_base64": content, "provenance": map[string]any{"origin": "json", "client": "test", "model": "model-b"}}
	runJSON("json-create", "document.create", project, input)
	input = map[string]any{"document_id": "json-doc", "revision_id": "json-r2", "expected_revision_id": "json-r1", "content_base64": content, "provenance": map[string]any{"origin": "json"}}
	runJSON("json-update", "document.update", project, input)
	runJSON("json-list", "document.list", project, map[string]any{})
	read := runJSON("json-read", "document.read", project, map[string]any{"document_id": "json-doc", "revision_id": "json-r2"})
	if got := read["result"].(map[string]any)["content_base64"]; got != content {
		t.Fatalf("JSON read base64 = %q, want original exact bytes %q", got, content)
	}
	runJSON("json-history", "document.history", project, map[string]any{"document_id": "json-doc"})
}

func TestDocumentCLIProcessRecordsExactNewlineDelimitedResponseBytes(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	library := t.TempDir()
	run := func(request map[string]any) []byte {
		t.Helper()
		raw, _ := json.Marshal(request)
		command := exec.Command(binary, "--library", library, "--json")
		command.Stdin = bytes.NewReader(raw)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("document CLI = %v/%q", err, output)
		}
		return output
	}
	run(map[string]any{"contract_version": "memgraphai.experimental/v1alpha1", "operation_id": "document-bytes-project", "operation": "project.create", "scope": map[string]any{"kind": "library"}, "input": map[string]any{"project_id": "project-bytes", "name": "Bytes"}})
	output := run(map[string]any{"contract_version": "memgraphai.experimental/v1alpha1", "operation_id": "document-bytes", "operation": "document.create", "scope": map[string]any{"kind": "project", "project_id": "project-bytes"}, "input": map[string]any{"document_id": "document-bytes", "revision_id": "revision-bytes", "expected_revision_id": nil, "content_base64": "AP8", "provenance": map[string]any{"origin": "cli"}}})
	if !bytes.HasSuffix(output, []byte{'\n'}) {
		t.Fatalf("document CLI output = %q, want newline-delimited JSON", output)
	}
	db, err := sql.Open("sqlite", filepath.Join(library, "database.sqlite"))
	if err != nil {
		t.Fatalf("open metrics database: %v", err)
	}
	defer db.Close()
	var recorded int
	if err := db.QueryRow("SELECT response_bytes FROM operation_metrics WHERE operation_id = 'document-bytes'").Scan(&recorded); err != nil {
		t.Fatalf("read document metric: %v", err)
	}
	if recorded != len(output) {
		t.Fatalf("document response_bytes = %d, emitted bytes = %d; want exact newline-delimited response", recorded, len(output))
	}
}

func TestDocumentCLIReplayRemainsReachableAfterDiscardedResponse(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	library := t.TempDir()
	run := func(raw []byte) map[string]any {
		t.Helper()
		command := exec.Command(binary, "--library", library, "--json")
		command.Stdin = bytes.NewReader(raw)
		output, err := command.CombinedOutput()
		var response map[string]any
		if decodeErr := json.Unmarshal(output, &response); decodeErr != nil {
			t.Fatalf("decode response %q: %v", output, decodeErr)
		}
		if response["outcome"].(map[string]any)["code"] != "ok" || err != nil {
			t.Fatalf("CLI result = %v/%#v, want durable success", err, response)
		}
		return response
	}
	project, _ := json.Marshal(map[string]any{"contract_version": "memgraphai.experimental/v1alpha1", "operation_id": "replay-project", "operation": "project.create", "scope": map[string]any{"kind": "library"}, "input": map[string]any{"project_id": "project-replay", "name": "Replay"}})
	run(project)
	write, _ := json.Marshal(map[string]any{"contract_version": "memgraphai.experimental/v1alpha1", "operation_id": "replay-document", "operation": "document.create", "scope": map[string]any{"kind": "project", "project_id": "project-replay"}, "input": map[string]any{"document_id": "document-replay", "revision_id": "revision-replay", "expected_revision_id": nil, "content_base64": "AP8", "provenance": map[string]any{"origin": "cli"}}})
	first := exec.Command(binary, "--library", library, "--json")
	first.Stdin = bytes.NewReader(write)
	first.Stdout = io.Discard
	var stderr bytes.Buffer
	first.Stderr = &stderr
	if err := first.Run(); err != nil {
		t.Fatalf("first separate process = %v/%q, want committed response that this test discards", err, stderr.String())
	}
	replayed := run(write)
	if got := replayed["result"].(map[string]any)["revision_id"]; got != "revision-replay" {
		t.Fatalf("replayed revision = %q, want durable stored revision", got)
	}
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
