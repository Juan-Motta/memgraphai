package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"memgraphai/internal/app"
	"memgraphai/internal/revisionfs"
	"memgraphai/internal/store/sqlite"
)

func TestStdioServerListsAndCallsProjectTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "memgraphai")
	build := exec.Command("go", "build", "-o", binary, "../../../cmd/memgraphai")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, output)
	}
	client := gomcp.NewClient(&gomcp.Implementation{Name: "projects-test", Version: "v1alpha1"}, nil)
	library := t.TempDir()
	command := exec.CommandContext(ctx, binary, "mcp", "--library", library)
	session, err := client.Connect(ctx, &gomcp.CommandTransport{Command: command, TerminateDuration: 250 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		_ = session.Wait()
	})
	tools, err := session.ListTools(t.Context(), nil)
	if err != nil || len(tools.Tools) != 21 {
		t.Fatalf("ListTools() = %d, %v; want twenty-one project, continuity, document, and checkpoint tools", len(tools.Tools), err)
	}
	path := filepath.Join(t.TempDir(), "associated")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("create path fixture: %v", err)
	}
	invoke := func(name string, args map[string]any) string {
		t.Helper()
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		called, err := session.CallTool(callCtx, &gomcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || called.IsError {
			t.Fatalf("CallTool(%s) error/result = %v/%t", name, err, called != nil && called.IsError)
		}
		output, ok := called.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("CallTool(%s) structured content = %T, want map", name, called.StructuredContent)
		}
		return output["outcome"].(map[string]any)["code"].(string)
	}
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"project.create", map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-create-a", "scope": map[string]any{"kind": "library"}, "input": map[string]any{"project_id": "project-a", "name": "Alpha"}}},
		{"project.create", map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-create-b", "scope": map[string]any{"kind": "library"}, "input": map[string]any{"project_id": "project-b", "name": "Beta"}}},
		{"project.list", map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-list", "scope": map[string]any{"kind": "library"}, "input": map[string]any{}}},
		{"project.association.add", map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-add", "scope": map[string]any{"kind": "project", "project_id": "project-a"}, "input": map[string]any{"path": path}}},
		{"project.association.list", map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-associations", "scope": map[string]any{"kind": "project", "project_id": "project-a"}, "input": map[string]any{}}},
		{"project.resolve", map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-resolve", "scope": map[string]any{"kind": "library"}, "input": map[string]any{"path": path}}},
	} {
		if code := invoke(call.name, call.args); code != app.OK {
			t.Fatalf("CallTool(%s) outcome = %q, want ok", call.name, code)
		}
	}
	paged, err := session.CallTool(t.Context(), &gomcp.CallToolParams{Name: "project.list", Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-page", "scope": map[string]any{"kind": "library"}, "input": map[string]any{}, "page": map[string]any{"limit": 1}}})
	if err != nil || paged.IsError {
		t.Fatalf("CallTool(project.list page) = %v/%t", err, paged != nil && paged.IsError)
	}
	pagedOutput, _ := paged.StructuredContent.(map[string]any)
	if pagedOutput["page"] == nil {
		t.Fatal("MCP paged list has no continuation page")
	}
	arguments := func(id, scope string, input map[string]any) map[string]any {
		value := map[string]any{"contract_version": app.ContractVersion, "operation_id": id, "scope": map[string]any{"kind": scope}, "input": input}
		return value
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatalf("create alias fixture: %v", err)
	}
	if code := invoke("project.resolve", arguments("mcp-alias", "library", map[string]any{"path": alias})); code != app.OK {
		t.Fatalf("alias outcome = %q, want ok", code)
	}
	nested := filepath.Join(path, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("create nested fixture: %v", err)
	}
	if code := invoke("project.resolve", arguments("mcp-nested", "library", map[string]any{"path": nested})); code != app.NotFound {
		t.Fatalf("nested outcome = %q, want not_found", code)
	}
	second := arguments("mcp-add-b", "project", map[string]any{"path": path})
	second["scope"] = map[string]any{"kind": "project", "project_id": "project-b"}
	if code := invoke("project.association.add", second); code != app.OK {
		t.Fatalf("second association outcome = %q, want ok", code)
	}
	if code := invoke("project.resolve", arguments("mcp-ambiguous", "library", map[string]any{"path": path})); code != app.Ambiguous {
		t.Fatalf("ambiguous outcome = %q, want ambiguous", code)
	}
	if code := invoke("project.association.list", arguments("mcp-missing-scope", "library", map[string]any{})); code != app.ScopeDenied {
		t.Fatalf("missing scope outcome = %q, want scope_denied", code)
	}
	secondPath := filepath.Join(t.TempDir(), "associated-2")
	if err := os.Mkdir(secondPath, 0o755); err != nil {
		t.Fatalf("create second association: %v", err)
	}
	if code := invoke("project.association.add", map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-add-a-2", "scope": map[string]any{"kind": "project", "project_id": "project-a"}, "input": map[string]any{"path": secondPath}}); code != app.OK {
		t.Fatalf("second project-a association outcome = %q", code)
	}
	associationPage, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "project.association.list", Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-association-page", "scope": map[string]any{"kind": "project", "project_id": "project-a"}, "input": map[string]any{}, "page": map[string]any{"limit": 1}}})
	if err != nil || associationPage.IsError {
		t.Fatalf("MCP association page = %v/%t", err, associationPage != nil && associationPage.IsError)
	}
	associationOutput := associationPage.StructuredContent.(map[string]any)
	token := associationOutput["page"].(map[string]any)["next_token"]
	if code := invoke("project.association.list", map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-association-next", "scope": map[string]any{"kind": "project", "project_id": "project-a"}, "input": map[string]any{}, "page": map[string]any{"limit": 1, "token": token}}); code != app.OK {
		t.Fatalf("MCP association continuation outcome = %q", code)
	}
	moved := path + "-moved"
	if err := os.Rename(path, moved); err != nil {
		t.Fatalf("move path fixture: %v", err)
	}
	if code := invoke("project.resolve", arguments("mcp-unavailable", "library", map[string]any{"path": path})); code != app.NotFound {
		t.Fatalf("unavailable outcome = %q, want not_found", code)
	}
	remove := arguments("mcp-remove", "project", map[string]any{"path": path})
	remove["scope"] = map[string]any{"kind": "project", "project_id": "project-a"}
	if code := invoke("project.association.remove", remove); code != app.OK {
		t.Fatalf("remove outcome = %q, want ok", code)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(library, "database.sqlite"))
	if err != nil {
		t.Fatalf("open metrics database: %v", err)
	}
	defer db.Close()
	var responseBytes int
	if err := db.QueryRowContext(ctx, "SELECT response_bytes FROM operation_metrics WHERE operation_id = 'mcp-create-a'").Scan(&responseBytes); err != nil || responseBytes == 0 {
		t.Fatalf("read MCP metric = %d, %v; want positive frame bytes", responseBytes, err)
	}
}

func TestStdioServerListsAndCallsContinuityTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, "../../../cmd/memgraphai").CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, output)
	}
	library := t.TempDir()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "continuity-test", Version: "v1alpha1"}, nil)
	command := exec.CommandContext(ctx, binary, "mcp", "--library", library)
	session, err := client.Connect(ctx, &gomcp.CommandTransport{Command: command, TerminateDuration: 250 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		_ = session.Wait()
	})
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools.Tools) != 21 {
		t.Fatalf("ListTools() = %d tools, want 21 project, continuity, document, and checkpoint tools", len(tools.Tools))
	}
	call := func(name, id string, scope, input map[string]any) map[string]any {
		t.Helper()
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		called, err := session.CallTool(callCtx, &gomcp.CallToolParams{Name: name, Arguments: map[string]any{
			"contract_version": app.ContractVersion, "operation_id": id, "scope": scope, "input": input,
		}})
		if err != nil || called.IsError {
			t.Fatalf("CallTool(%s) = %v/%t", name, err, called != nil && called.IsError)
		}
		output, ok := called.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("CallTool(%s) content = %T, want map", name, called.StructuredContent)
		}
		return output
	}
	assert := func(want, name, id string, scope, input map[string]any) map[string]any {
		output := call(name, id, scope, input)
		if code := output["outcome"].(map[string]any)["code"]; code != want {
			t.Fatalf("CallTool(%s) outcome = %q, want %q", name, code, want)
		}
		return output
	}
	libraryScope := map[string]any{"kind": "library"}
	projectScope := map[string]any{"kind": "project", "project_id": "project-a"}
	workstreamScope := map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "workstream-a"}
	sessionScope := map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-a", "session_id": "session-a"}
	assert("ok", "project.create", "mcp-continuity-project", libraryScope, map[string]any{"project_id": "project-a", "name": "Alpha"})
	assert("ok", "workstream.create", "mcp-continuity-create", projectScope, map[string]any{"workstream_id": "workstream-a", "origin": "mcp"})
	assert("ok", "workstream.list", "mcp-continuity-list", projectScope, map[string]any{})
	assert("ok", "workstream.fork", "mcp-continuity-fork", workstreamScope, map[string]any{"workstream_id": "workstream-fork", "origin": "mcp"})
	assert("ok", "session.open", "mcp-continuity-open", workstreamScope, map[string]any{"session_id": "session-a", "origin": "mcp"})
	assert("ok", "session.status", "mcp-continuity-status", sessionScope, map[string]any{})
	assert("ok", "session.disconnect", "mcp-continuity-disconnect", sessionScope, map[string]any{"observed_by": "mcp-client"})
	assert("ok", "session.close", "mcp-continuity-close", sessionScope, map[string]any{})
	closed := assert("ok", "session.status", "mcp-continuity-closed", sessionScope, map[string]any{})
	if status := closed["result"].(map[string]any)["status"]; status != "closed" {
		t.Fatalf("closed status = %q, want closed", status)
	}
	resumed := assert("ok", "session.resume", "mcp-continuity-resume", workstreamScope, map[string]any{"session_id": "session-resumed", "source_session_id": "session-a", "origin": "mcp"})
	if source := resumed["result"].(map[string]any)["resumed_from_session_id"]; source != "session-a" {
		t.Fatalf("resume source = %q, want session-a", source)
	}
	wrongScope := map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-fork", "session_id": "session-a"}
	assert("binding_mismatch", "session.status", "mcp-continuity-wrong-scope", wrongScope, map[string]any{})
}

func TestContinuityAdaptersHaveEquivalentOutcomes(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, "../../../cmd/memgraphai").CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, output)
	}
	cliLibrary, mcpLibrary := t.TempDir(), t.TempDir()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "continuity-parity", Version: "v1alpha1"}, nil)
	session, err := client.Connect(ctx, &gomcp.CommandTransport{Command: exec.CommandContext(ctx, binary, "mcp", "--library", mcpLibrary), TerminateDuration: 250 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("connect MCP: %v", err)
	}
	defer func() {
		_ = session.Close()
		_ = session.Wait()
	}()
	invokeMCP := func(operation string, request map[string]any) map[string]any {
		t.Helper()
		called, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: operation, Arguments: request})
		if err != nil || called.IsError {
			t.Fatalf("MCP %s = %v/%t", operation, err, called != nil && called.IsError)
		}
		output, ok := called.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("MCP %s output = %T, want map", operation, called.StructuredContent)
		}
		return output
	}
	requests := []map[string]any{
		{"contract_version": app.ContractVersion, "operation_id": "parity-project", "operation": "project.create", "scope": map[string]any{"kind": "library"}, "input": map[string]any{"project_id": "project-a", "name": "Alpha"}},
		{"contract_version": app.ContractVersion, "operation_id": "parity-workstream", "operation": "workstream.create", "scope": map[string]any{"kind": "project", "project_id": "project-a"}, "input": map[string]any{"workstream_id": "workstream-a", "origin": "parity"}},
		{"contract_version": app.ContractVersion, "operation_id": "parity-list", "operation": "workstream.list", "scope": map[string]any{"kind": "project", "project_id": "project-a"}, "input": map[string]any{}},
		{"contract_version": app.ContractVersion, "operation_id": "parity-fork", "operation": "workstream.fork", "scope": map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "workstream-a"}, "input": map[string]any{"workstream_id": "workstream-fork", "origin": "parity"}},
		{"contract_version": app.ContractVersion, "operation_id": "parity-open", "operation": "session.open", "scope": map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "workstream-a"}, "input": map[string]any{"session_id": "session-a", "origin": "parity"}},
		{"contract_version": app.ContractVersion, "operation_id": "parity-status", "operation": "session.status", "scope": map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-a", "session_id": "session-a"}, "input": map[string]any{}},
		{"contract_version": app.ContractVersion, "operation_id": "parity-disconnect", "operation": "session.disconnect", "scope": map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-a", "session_id": "session-a"}, "input": map[string]any{"observed_by": "parity-client"}},
		{"contract_version": app.ContractVersion, "operation_id": "parity-close", "operation": "session.close", "scope": map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-a", "session_id": "session-a"}, "input": map[string]any{}},
		{"contract_version": app.ContractVersion, "operation_id": "parity-resume", "operation": "session.resume", "scope": map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "workstream-a"}, "input": map[string]any{"session_id": "session-resumed", "source_session_id": "session-a", "origin": "parity"}},
		{"contract_version": app.ContractVersion, "operation_id": "parity-mismatch", "operation": "session.status", "scope": map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-fork", "session_id": "session-a"}, "input": map[string]any{}},
		{"contract_version": app.ContractVersion, "operation_id": "parity-missing", "operation": "session.status", "scope": map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-a", "session_id": "session-missing"}, "input": map[string]any{}},
	}
	for _, request := range requests {
		raw, _ := json.Marshal(request)
		command := exec.CommandContext(ctx, binary, "--library", cliLibrary, "--json")
		command.Stdin = bytes.NewReader(raw)
		cliWire, cliErr := command.CombinedOutput()
		var cliOutput map[string]any
		if err := json.Unmarshal(cliWire, &cliOutput); err != nil {
			t.Fatalf("decode CLI %s response %q: %v", request["operation"], cliWire, err)
		}
		mcpInput := map[string]any{"contract_version": request["contract_version"], "operation_id": request["operation_id"], "scope": request["scope"], "input": request["input"]}
		mcpOutput := invokeMCP(request["operation"].(string), mcpInput)
		if (cliOutput["outcome"].(map[string]any)["code"] == app.OK) != (cliErr == nil) {
			t.Fatalf("CLI %s exit/result disagree: %v/%#v", request["operation"], cliErr, cliOutput)
		}
		if got, want := continuitySemantic(cliOutput), continuitySemantic(mcpOutput); !reflect.DeepEqual(got, want) {
			t.Fatalf("adapter parity for %s\nCLI: %#v\nMCP: %#v", request["operation"], got, want)
		}
	}
}

func continuitySemantic(value map[string]any) map[string]any {
	encoded, _ := json.Marshal(value)
	var copy map[string]any
	_ = json.Unmarshal(encoded, &copy)
	var scrub func(any)
	scrub = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			delete(typed, "created_at")
			delete(typed, "opened_at")
			for _, child := range typed {
				scrub(child)
			}
		case []any:
			for _, child := range typed {
				scrub(child)
			}
		}
	}
	scrub(copy)
	return copy
}

func TestContinuityMCPEOFLeavesSessionsUnchangedAndDisconnectIsScoped(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, "../../../cmd/memgraphai").CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, output)
	}
	library := t.TempDir()
	runCLI := func(id, operation string, scope, input map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"contract_version": app.ContractVersion, "operation_id": id, "operation": operation, "scope": scope, "input": input})
		command := exec.CommandContext(ctx, binary, "--library", library, "--json")
		command.Stdin = bytes.NewReader(raw)
		output, err := command.CombinedOutput()
		var response map[string]any
		if decodeErr := json.Unmarshal(output, &response); decodeErr != nil {
			t.Fatalf("decode CLI %s response %q: %v", operation, output, decodeErr)
		}
		if code := response["outcome"].(map[string]any)["code"]; (code == app.OK) != (err == nil) {
			t.Fatalf("CLI %s = %v/%#v", operation, err, response)
		}
		return response
	}
	projectA := map[string]any{"kind": "project", "project_id": "project-a"}
	projectB := map[string]any{"kind": "project", "project_id": "project-b"}
	workstreamA := map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "workstream-a"}
	workstreamB := map[string]any{"kind": "workstream", "project_id": "project-b", "workstream_id": "workstream-b"}
	runCLI("eof-project-a", "project.create", map[string]any{"kind": "library"}, map[string]any{"project_id": "project-a", "name": "Alpha"})
	runCLI("eof-project-b", "project.create", map[string]any{"kind": "library"}, map[string]any{"project_id": "project-b", "name": "Beta"})
	runCLI("eof-workstream-a", "workstream.create", projectA, map[string]any{"workstream_id": "workstream-a", "origin": "eof"})
	runCLI("eof-workstream-b", "workstream.create", projectB, map[string]any{"workstream_id": "workstream-b", "origin": "eof"})
	runCLI("eof-open-a", "session.open", workstreamA, map[string]any{"session_id": "session-a", "origin": "eof"})
	runCLI("eof-open-b", "session.open", workstreamB, map[string]any{"session_id": "session-b", "origin": "eof"})
	connect := func() *gomcp.ClientSession {
		t.Helper()
		client := gomcp.NewClient(&gomcp.Implementation{Name: "eof-test", Version: "v1alpha1"}, nil)
		session, err := client.Connect(ctx, &gomcp.CommandTransport{Command: exec.CommandContext(ctx, binary, "mcp", "--library", library), TerminateDuration: 250 * time.Millisecond}, nil)
		if err != nil {
			t.Fatalf("connect MCP: %v", err)
		}
		return session
	}
	rawEOF := connect()
	if err := rawEOF.Close(); err != nil {
		t.Fatalf("close raw EOF session: %v", err)
	}
	if err := rawEOF.Wait(); err != nil {
		t.Fatalf("wait raw EOF session: %v", err)
	}
	status := func(id, project, workstream, sessionID string) string {
		response := runCLI(id, "session.status", map[string]any{"kind": "session", "project_id": project, "workstream_id": workstream, "session_id": sessionID}, map[string]any{})
		return response["result"].(map[string]any)["status"].(string)
	}
	if got := status("eof-status-a", "project-a", "workstream-a", "session-a"); got != "open" {
		t.Fatalf("raw EOF session-a status = %q, want open", got)
	}
	if got := status("eof-status-b", "project-b", "workstream-b", "session-b"); got != "open" {
		t.Fatalf("raw EOF session-b status = %q, want open", got)
	}
	attributed := connect()
	interleaved, err := attributed.CallTool(ctx, &gomcp.CallToolParams{Name: "session.status", Arguments: map[string]any{
		"contract_version": app.ContractVersion, "operation_id": "eof-interleaved-b", "scope": map[string]any{"kind": "session", "project_id": "project-b", "workstream_id": "workstream-b", "session_id": "session-b"}, "input": map[string]any{},
	}})
	if err != nil || interleaved.IsError || interleaved.StructuredContent.(map[string]any)["result"].(map[string]any)["status"] != "open" {
		t.Fatalf("interleaved project-b status = %v/%#v, want open", err, interleaved)
	}
	_, err = attributed.CallTool(ctx, &gomcp.CallToolParams{Name: "session.disconnect", Arguments: map[string]any{
		"contract_version": app.ContractVersion, "operation_id": "eof-disconnect-a", "scope": map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-a", "session_id": "session-a"}, "input": map[string]any{"observed_by": "mcp-observer"},
	}})
	if err != nil {
		t.Fatalf("explicit disconnect: %v", err)
	}
	_ = attributed.Close()
	if err := attributed.Wait(); err != nil {
		t.Fatalf("wait attributed session: %v", err)
	}
	if got := status("eof-disconnected-a", "project-a", "workstream-a", "session-a"); got != "disconnected" {
		t.Fatalf("explicitly disconnected session-a = %q, want disconnected", got)
	}
	if got := status("eof-unchanged-b", "project-b", "workstream-b", "session-b"); got != "open" {
		t.Fatalf("unselected session-b = %q, want open", got)
	}
}

func TestStdioServerExposesDocumentToolsAndExactBytes(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, "../../../cmd/memgraphai").CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, output)
	}
	library := t.TempDir()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "documents-test", Version: "v1alpha1"}, nil)
	session, err := client.Connect(ctx, &gomcp.CommandTransport{Command: exec.CommandContext(ctx, binary, "mcp", "--library", library), TerminateDuration: 250 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("connect MCP: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		_ = session.Wait()
	})
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 21 {
		t.Fatalf("ListTools() = %d, %v; want twenty-one project, continuity, document, and checkpoint tools", len(tools.Tools), err)
	}
	call := func(name, id string, scope, input map[string]any) map[string]any {
		t.Helper()
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		called, err := session.CallTool(callCtx, &gomcp.CallToolParams{Name: name, Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": id, "scope": scope, "input": input}})
		if err != nil || called.IsError {
			t.Fatalf("CallTool(%s) = %v/%t", name, err, called != nil && called.IsError)
		}
		return called.StructuredContent.(map[string]any)
	}
	libraryScope := map[string]any{"kind": "library"}
	projectScope := map[string]any{"kind": "project", "project_id": "project-a"}
	workstreamScope := map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "stream-a"}
	call("project.create", "mcp-document-project", libraryScope, map[string]any{"project_id": "project-a", "name": "Alpha"})
	call("workstream.create", "mcp-document-stream", projectScope, map[string]any{"workstream_id": "stream-a", "origin": "mcp"})
	content := "AP9NYXJrZG93bgo"
	created := call("document.create", "mcp-document-create", projectScope, map[string]any{"document_id": "document-a", "revision_id": "revision-1", "expected_revision_id": nil, "content_base64": content, "provenance": map[string]any{"origin": "mcp", "client": "sdk", "model": "model-a"}})
	if got := created["result"].(map[string]any)["provenance"].(map[string]any)["model"]; got != "model-a" {
		t.Fatalf("create provenance model = %q, want model-a", got)
	}
	call("document.update", "mcp-document-update", projectScope, map[string]any{"document_id": "document-a", "revision_id": "revision-2", "expected_revision_id": "revision-1", "content_base64": content, "provenance": map[string]any{"origin": "mcp"}})
	listed := call("document.list", "mcp-document-list", projectScope, map[string]any{})
	if len(listed["result"].(map[string]any)["documents"].([]any)) != 1 {
		t.Fatalf("document list = %#v, want one project-general document", listed)
	}
	read := call("document.read", "mcp-document-read", projectScope, map[string]any{"document_id": "document-a", "revision_id": "revision-2"})
	if got := read["result"].(map[string]any)["content_base64"]; got != content {
		t.Fatalf("MCP read base64 = %q, want exact original %q", got, content)
	}
	history := call("document.history", "mcp-document-history", projectScope, map[string]any{"document_id": "document-a"})
	if len(history["result"].(map[string]any)["revisions"].([]any)) != 2 {
		t.Fatalf("document history = %#v, want two exact revisions", history)
	}
	wrongScope := call("document.read", "mcp-document-wrong-scope", workstreamScope, map[string]any{"document_id": "document-a"})
	if got := wrongScope["outcome"].(map[string]any)["code"]; got != app.ScopeDenied {
		t.Fatalf("workstream read of project-general document = %q, want %q", got, app.ScopeDenied)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close first MCP connection: %v", err)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("wait first MCP connection: %v", err)
	}
	reconnected, err := client.Connect(ctx, &gomcp.CommandTransport{Command: exec.CommandContext(ctx, binary, "mcp", "--library", library), TerminateDuration: 250 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("reconnect MCP: %v", err)
	}
	t.Cleanup(func() {
		_ = reconnected.Close()
		_ = reconnected.Wait()
	})
	replayed, err := reconnected.CallTool(ctx, &gomcp.CallToolParams{Name: "document.create", Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": "mcp-document-create", "scope": projectScope, "input": map[string]any{"document_id": "document-a", "revision_id": "revision-1", "expected_revision_id": nil, "content_base64": content, "provenance": map[string]any{"origin": "mcp", "client": "sdk", "model": "model-a"}}}})
	if err != nil || replayed.IsError || replayed.StructuredContent.(map[string]any)["result"].(map[string]any)["revision_id"] != "revision-1" {
		t.Fatalf("reconnected identical replay = %v/%#v, want stored durable revision", err, replayed)
	}
}

func TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame(t *testing.T) {
	for _, test := range []struct {
		name, operationID, operation string
		scope, input                 map[string]any
		setup                        func(context.Context, *gomcp.ClientSession)
	}{
		{"project", "wire-project", "project.create", map[string]any{"kind": "library"}, map[string]any{"project_id": "project-wire", "name": "Wire"}, nil},
		{"document", "wire-document", "document.create", map[string]any{"kind": "project", "project_id": "project-wire"}, map[string]any{"document_id": "document-wire", "revision_id": "revision-wire", "expected_revision_id": nil, "content_base64": "d2lyZQ", "provenance": map[string]any{"origin": "wire"}}, func(ctx context.Context, session *gomcp.ClientSession) {
			_, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "project.create", Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": "fixture-project", "scope": map[string]any{"kind": "library"}, "input": map[string]any{"project_id": "project-wire", "name": "Wire"}}})
			if err != nil {
				t.Fatalf("document fixture project: %v", err)
			}
		}},
		{"checkpoint", "wire-checkpoint", "checkpoint.save", map[string]any{"kind": "session", "project_id": "project-wire", "workstream_id": "workstream-wire", "session_id": "session-wire"}, map[string]any{"checkpoint_id": "checkpoint-wire", "checkpoint_document_id": "checkpoint-document-wire", "checkpoint_revision_id": "checkpoint-revision-wire", "prose_base64": "d2lyZQ", "references": []map[string]any{{"document_id": "source-wire", "revision_id": "source-revision-wire"}}, "provenance": map[string]any{"origin": "wire"}}, func(ctx context.Context, session *gomcp.ClientSession) {
			call := func(name, id string, scope, input map[string]any) {
				t.Helper()
				called, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: name, Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": id, "scope": scope, "input": input}})
				if err != nil || called.IsError {
					t.Fatalf("checkpoint metric fixture %s = %v/%t", name, err, called != nil && called.IsError)
				}
			}
			call("project.create", "wire-checkpoint-project", map[string]any{"kind": "library"}, map[string]any{"project_id": "project-wire", "name": "Wire"})
			call("workstream.create", "wire-checkpoint-workstream", map[string]any{"kind": "project", "project_id": "project-wire"}, map[string]any{"workstream_id": "workstream-wire", "origin": "wire"})
			call("session.open", "wire-checkpoint-session", map[string]any{"kind": "workstream", "project_id": "project-wire", "workstream_id": "workstream-wire"}, map[string]any{"session_id": "session-wire", "origin": "wire"})
			call("document.create", "wire-checkpoint-source", map[string]any{"kind": "project", "project_id": "project-wire"}, map[string]any{"document_id": "source-wire", "revision_id": "source-revision-wire", "expected_revision_id": nil, "content_base64": "d2lyZQ", "provenance": map[string]any{"origin": "wire"}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			storePath := filepath.Join(t.TempDir(), "database.sqlite")
			store, err := sqlite.Open(ctx, storePath)
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			serverIn, clientOut := io.Pipe()
			clientIn, serverOut := io.Pipe()
			capture := &frameCapture{WriteCloser: serverOut}
			serverDone := make(chan error, 1)
			go func() {
				files := revisionfs.New(filepath.Dir(storePath), nil)
				serverDone <- RunIO(ctx, app.ProjectService{Store: store}, app.ContinuityService{Store: store}, app.DocumentService{Store: store, Files: files}, app.Service{Recorder: store}, serverIn, capture, app.CheckpointService{Store: store, Files: files})
			}()
			session, err := gomcp.NewClient(&gomcp.Implementation{Name: "wire-test", Version: "v1alpha1"}, nil).Connect(ctx, &gomcp.IOTransport{Reader: clientIn, Writer: clientOut}, nil)
			if err != nil {
				t.Fatalf("connect: %v", err)
			}
			if test.setup != nil {
				test.setup(ctx, session)
			}
			_, err = session.CallTool(ctx, &gomcp.CallToolParams{Name: test.operation, Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": test.operationID, "scope": test.scope, "input": test.input}})
			if err != nil {
				t.Fatalf("CallTool(%s): %v", test.operation, err)
			}
			_ = session.Close()
			select {
			case <-serverDone:
			case <-ctx.Done():
				t.Fatalf("server shutdown exceeded deadline: %v", ctx.Err())
			}
			if err := store.Close(); err != nil {
				t.Fatalf("close store: %v", err)
			}
			db, err := sql.Open("sqlite", storePath)
			if err != nil {
				t.Fatalf("open metric database: %v", err)
			}
			defer db.Close()
			frame := capture.frameContaining(test.operationID)
			var recorded int
			if len(frame) == 0 {
				t.Fatalf("captured JSON-RPC frame for %q is empty", test.operationID)
			}
			if err := db.QueryRowContext(ctx, "SELECT response_bytes FROM operation_metrics WHERE operation_id = ?", test.operationID).Scan(&recorded); err != nil {
				t.Fatalf("read %s metric: %v", test.operationID, err)
			}
			if recorded != len(frame) {
				t.Fatalf("%s response_bytes = %d, captured frame bytes = %d; want exact equality", test.operationID, recorded, len(frame))
			}
		})
	}
}

type frameCapture struct {
	io.WriteCloser
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *frameCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	_, _ = c.buf.Write(p)
	c.mu.Unlock()
	return c.WriteCloser.Write(p)
}

func (c *frameCapture) frameContaining(operationID string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, frame := range bytes.SplitAfter(c.buf.Bytes(), []byte{'\n'}) {
		var response any
		if json.Unmarshal(frame, &response) != nil {
			continue
		}
		for _, id := range responseOperationIDs(response) {
			if id == operationID {
				return append([]byte(nil), frame...)
			}
		}
	}
	return nil
}

func TestDocumentAdaptersHaveEquivalentSemanticEnvelopes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, "../../../cmd/memgraphai").CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, output)
	}
	cliRoot, mcpRoot := t.TempDir(), t.TempDir()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "documents-parity", Version: "v1alpha1"}, nil)
	mcpSession, err := client.Connect(ctx, &gomcp.CommandTransport{Command: exec.CommandContext(ctx, binary, "mcp", "--library", mcpRoot), TerminateDuration: 250 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("connect MCP: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		if err := mcpSession.Close(); err != nil {
			t.Errorf("close MCP: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- mcpSession.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("wait MCP: %v", err)
			}
		case <-cleanupCtx.Done():
			t.Errorf("MCP Close/Wait exceeded deadline: %v", cleanupCtx.Err())
		}
	})
	invokeCLI := func(operation, id string, scope, input map[string]any, page map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"contract_version": app.ContractVersion, "operation_id": id, "operation": operation, "scope": scope, "input": input, "page": page})
		command := exec.CommandContext(ctx, binary, "--library", cliRoot, "--json")
		command.Stdin = bytes.NewReader(raw)
		output, _ := command.CombinedOutput()
		var response map[string]any
		if err := json.Unmarshal(output, &response); err != nil {
			t.Fatalf("decode CLI %s response %q: %v", operation, output, err)
		}
		return response
	}
	invokeMCP := func(operation, id string, scope, input map[string]any, page map[string]any) map[string]any {
		t.Helper()
		called, err := mcpSession.CallTool(ctx, &gomcp.CallToolParams{Name: operation, Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": id, "scope": scope, "input": input, "page": page}})
		if err != nil || called.IsError {
			t.Fatalf("MCP %s = %v/%t", operation, err, called != nil && called.IsError)
		}
		response, ok := called.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("MCP %s response = %T, want map", operation, called.StructuredContent)
		}
		return response
	}
	compare := func(name, operation, id string, scope, input, page map[string]any) (map[string]any, map[string]any) {
		t.Helper()
		cli, mcp := invokeCLI(operation, id, scope, input, page), invokeMCP(operation, id, scope, input, page)
		if got, want := documentSemanticEnvelope(cli), documentSemanticEnvelope(mcp); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s semantic envelope mismatch\nCLI: %#v\nMCP: %#v", name, got, want)
		}
		return cli, mcp
	}
	library := map[string]any{"kind": "library"}
	project := map[string]any{"kind": "project", "project_id": "project-a"}
	workstream := map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "stream-a"}
	content := "AP9NYXJrZG93bgo"
	write := func(document, revision, expected, origin string) map[string]any {
		return map[string]any{"document_id": document, "revision_id": revision, "expected_revision_id": expected, "content_base64": content, "provenance": map[string]any{"origin": origin, "client": "parity", "model": "model-a"}}
	}
	writeNull := func(document, revision, origin string) map[string]any {
		value := write(document, revision, "", origin)
		value["expected_revision_id"] = nil
		return value
	}
	for _, test := range []struct {
		name, operation, id string
		scope, input, page  map[string]any
	}{
		{"project", "project.create", "parity-project", library, map[string]any{"project_id": "project-a", "name": "Alpha"}, nil},
		{"workstream", "workstream.create", "parity-workstream", project, map[string]any{"workstream_id": "stream-a", "origin": "parity"}, nil},
		{"project document", "document.create", "parity-project-create", project, writeNull("project-doc", "project-r1", "cli"), nil},
		{"workstream document", "document.create", "parity-workstream-create", workstream, writeNull("workstream-doc", "stream-r1", "mcp"), nil},
		{"foreign document", "document.create", "parity-foreign-create", project, writeNull("foreign-doc", "foreign-r1", "cli"), nil},
		{"project update", "document.update", "parity-project-update", project, write("project-doc", "project-r2", "project-r1", "cli"), nil},
	} {
		t.Run(test.name, func(t *testing.T) { compare(test.name, test.operation, test.id, test.scope, test.input, test.page) })
	}
	createdCLI, createdMCP := compare("identical create replay", "document.create", "parity-project-create", project, writeNull("project-doc", "project-r1", "cli"), nil)
	for _, response := range []map[string]any{createdCLI, createdMCP} {
		result := response["result"].(map[string]any)
		checksum := sha256.Sum256([]byte{0, 0xff, 'M', 'a', 'r', 'k', 'd', 'o', 'w', 'n', '\n'})
		if result["checksum"] != fmt.Sprintf("%x", checksum) || result["byte_count"] != float64(11) || result["provenance"].(map[string]any)["model"] != "model-a" {
			t.Fatalf("create metadata = %#v, want exact checksum, byte count, and provenance", result)
		}
	}
	matrix := []struct {
		name, operation, id string
		scope, input, page  map[string]any
		want                string
	}{
		{"project-general excludes workstream", "document.list", "parity-project-list", project, map[string]any{}, nil, app.OK},
		{"workstream excludes project-general", "document.list", "parity-workstream-list", workstream, map[string]any{}, nil, app.OK},
		{"foreign historical revision binding", "document.read", "parity-foreign-history", project, map[string]any{"document_id": "project-doc", "revision_id": "foreign-r1"}, nil, app.BindingMismatch},
		{"wrong-scope read", "document.read", "parity-wrong-scope-read", workstream, map[string]any{"document_id": "project-doc"}, nil, app.ScopeDenied},
		{"stale update preserves current", "document.update", "parity-stale", project, write("project-doc", "project-r3", "project-r1", "cli"), nil, app.Conflict},
		{"changed immutable replay mismatch", "document.create", "parity-project-create", project, writeNull("project-doc", "project-r1", "changed"), nil, app.IdempotencyMismatch},
		{"zero list limit", "document.list", "parity-limit-zero", project, map[string]any{}, map[string]any{"limit": 0}, app.Invalid},
		{"negative list limit", "document.list", "parity-limit-negative", project, map[string]any{}, map[string]any{"limit": -1}, app.Invalid},
		{"over-max list limit", "document.list", "parity-limit-over", project, map[string]any{}, map[string]any{"limit": 201}, app.Invalid},
		{"zero history limit", "document.history", "parity-history-limit-zero", project, map[string]any{"document_id": "project-doc"}, map[string]any{"limit": 0}, app.Invalid},
		{"negative history limit", "document.history", "parity-history-limit-negative", project, map[string]any{"document_id": "project-doc"}, map[string]any{"limit": -1}, app.Invalid},
		{"over-max history limit", "document.history", "parity-history-limit-over", project, map[string]any{"document_id": "project-doc"}, map[string]any{"limit": 201}, app.Invalid},
	}
	for _, test := range matrix {
		t.Run(test.name, func(t *testing.T) {
			cli, mcp := compare(test.name, test.operation, test.id, test.scope, test.input, test.page)
			for _, response := range []map[string]any{cli, mcp} {
				if got := response["outcome"].(map[string]any)["code"]; got != test.want {
					t.Fatalf("%s outcome = %q, want %q", test.name, got, test.want)
				}
			}
		})
	}
	projectListCLI, projectListMCP := compare("project-general default list", "document.list", "parity-project-default", project, map[string]any{}, nil)
	for _, response := range []map[string]any{projectListCLI, projectListMCP} {
		for _, item := range response["result"].(map[string]any)["documents"].([]any) {
			if _, found := item.(map[string]any)["workstream_id"]; found {
				t.Fatalf("project-general list widened into a workstream: %#v", response)
			}
		}
	}
	workstreamListCLI, workstreamListMCP := compare("workstream default list", "document.list", "parity-workstream-default", workstream, map[string]any{}, nil)
	for _, response := range []map[string]any{workstreamListCLI, workstreamListMCP} {
		documents := response["result"].(map[string]any)["documents"].([]any)
		if len(documents) != 1 || documents[0].(map[string]any)["document_id"] != "workstream-doc" {
			t.Fatalf("workstream list = %#v, want only its explicit document", response)
		}
	}
	currentCLI, currentMCP := compare("current remains revision two", "document.read", "parity-current", project, map[string]any{"document_id": "project-doc"}, nil)
	for _, response := range []map[string]any{currentCLI, currentMCP} {
		result := response["result"].(map[string]any)
		if result["revision_id"] != "project-r2" || result["content_base64"] != content {
			t.Fatalf("current after stale update = %#v, want revision two exact bytes", result)
		}
	}
	historicalCLI, historicalMCP := compare("historical exact byte parity", "document.read", "parity-historical-exact", project, map[string]any{"document_id": "project-doc", "revision_id": "project-r1"}, nil)
	for _, response := range []map[string]any{historicalCLI, historicalMCP} {
		result := response["result"].(map[string]any)
		if result["revision_id"] != "project-r1" || result["content_base64"] != content {
			t.Fatalf("historical read = %#v, want revision one exact bytes", result)
		}
	}
	listCLI, listMCP := compare("list continuation source", "document.list", "parity-list-page", project, map[string]any{}, map[string]any{"limit": 1})
	historyCLI, historyMCP := compare("history continuation source", "document.history", "parity-history-page", project, map[string]any{"document_id": "project-doc"}, map[string]any{"limit": 1})
	for _, pair := range [][3]any{{listCLI, listMCP, "document.list"}, {historyCLI, historyMCP, "document.history"}} {
		cli, mcp, operation := pair[0].(map[string]any), pair[1].(map[string]any), pair[2].(string)
		cliToken := cli["page"].(map[string]any)["next_token"].(string)
		mcpToken := mcp["page"].(map[string]any)["next_token"].(string)
		if cliToken == "" || mcpToken == "" {
			t.Fatalf("%s continuation token missing", operation)
		}
		input := map[string]any{}
		if operation == "document.history" {
			input["document_id"] = "project-doc"
		}
		cliFollow := invokeCLI(operation, "parity-"+operation+"-next", project, input, map[string]any{"limit": 1, "token": cliToken})
		mcpFollow := invokeMCP(operation, "parity-"+operation+"-next", project, input, map[string]any{"limit": 1, "token": mcpToken})
		if got, want := documentSemanticEnvelope(cliFollow), documentSemanticEnvelope(mcpFollow); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s own-token semantic mismatch\nCLI: %#v\nMCP: %#v", operation, got, want)
		}
		if got := mcpFollow["outcome"].(map[string]any)["code"]; got != app.OK {
			t.Fatalf("MCP %s own token outcome = %q, want ok", operation, got)
		}
		reboundInput := map[string]any{}
		if operation == "document.history" {
			reboundInput["document_id"] = "project-doc"
		}
		for _, response := range []map[string]any{invokeCLI(operation, "parity-rebound-cli", workstream, reboundInput, map[string]any{"limit": 1, "token": cliToken}), invokeMCP(operation, "parity-rebound-mcp", workstream, reboundInput, map[string]any{"limit": 1, "token": mcpToken})} {
			if got := response["outcome"].(map[string]any)["code"]; got != app.Invalid {
				t.Fatalf("%s rebound token outcome = %q, want invalid", operation, got)
			}
		}
	}
	compare("freshness document create", "document.create", "parity-freshness-create", project, writeNull("project-doc-2", "project-2-r1", "cli"), nil)
	compare("mutation invalidates continuation tokens", "document.update", "parity-freshness-update", project, write("project-doc-2", "project-2-r2", "project-2-r1", "cli"), nil)
	for _, test := range []struct {
		operation string
		input     map[string]any
		cliToken  string
		mcpToken  string
	}{
		{"document.list", map[string]any{}, listCLI["page"].(map[string]any)["next_token"].(string), listMCP["page"].(map[string]any)["next_token"].(string)},
		{"document.history", map[string]any{"document_id": "project-doc"}, historyCLI["page"].(map[string]any)["next_token"].(string), historyMCP["page"].(map[string]any)["next_token"].(string)},
	} {
		for _, response := range []map[string]any{invokeCLI(test.operation, "parity-stale-token-cli", project, test.input, map[string]any{"limit": 1, "token": test.cliToken}), invokeMCP(test.operation, "parity-stale-token-mcp", project, test.input, map[string]any{"limit": 1, "token": test.mcpToken})} {
			if got := response["outcome"].(map[string]any)["code"]; got != app.Conflict {
				t.Fatalf("stale %s token outcome = %q, want conflict", test.operation, got)
			}
		}
	}
	for _, root := range []string{cliRoot, mcpRoot} {
		for _, revision := range []string{"project-r2", "project-r1"} {
			path := filepath.Join(root, "projects", "project-a", "documents", "project-doc", "revisions", revision+".md")
			if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
				t.Fatalf("tamper temporary %s: %v", path, err)
			}
		}
	}
	for _, test := range []struct{ name, revision string }{{"current integrity", "project-r2"}, {"historical integrity", "project-r1"}} {
		t.Run(test.name, func(t *testing.T) {
			cli, mcp := compare(test.name, "document.read", "parity-"+test.revision, project, map[string]any{"document_id": "project-doc", "revision_id": test.revision}, nil)
			if cli["outcome"].(map[string]any)["code"] != app.IntegrityDiscrepancy || mcp["outcome"].(map[string]any)["code"] != app.IntegrityDiscrepancy {
				t.Fatalf("%s did not report integrity discrepancy", test.name)
			}
		})
	}
}

func documentSemanticEnvelope(value map[string]any) map[string]any {
	encoded, _ := json.Marshal(value)
	var copy map[string]any
	_ = json.Unmarshal(encoded, &copy)
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			delete(typed, "created_at")
			for _, child := range typed {
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(copy)
	return copy
}

func TestStdioServerExposesCheckpointSaveAndReadTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, "../../../cmd/memgraphai").CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, output)
	}
	client := gomcp.NewClient(&gomcp.Implementation{Name: "checkpoints-test", Version: "v1alpha1"}, nil)
	command := exec.CommandContext(ctx, binary, "mcp", "--library", t.TempDir())
	session, err := client.Connect(ctx, &gomcp.CommandTransport{Command: command, TerminateDuration: 250 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer func() {
		_ = session.Close()
		_ = session.Wait()
	}()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 21 {
		t.Fatalf("ListTools() = %d, %v; want twenty-one tools including checkpoint save/read", len(tools.Tools), err)
	}
}

func TestCheckpointAdaptersHaveEquivalentProcessOutcomes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	binary := filepath.Join(t.TempDir(), "memgraphai")
	if output, err := exec.Command("go", "build", "-o", binary, "../../../cmd/memgraphai").CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, output)
	}
	cliRoot, mcpRoot := t.TempDir(), t.TempDir()
	cliCall := func(id, operation string, scope, input map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"contract_version": app.ContractVersion, "operation_id": id, "operation": operation, "scope": scope, "input": input})
		command := exec.CommandContext(ctx, binary, "--library", cliRoot, "--json")
		command.Stdin = bytes.NewReader(raw)
		output, _ := command.CombinedOutput()
		var response map[string]any
		if err := json.Unmarshal(output, &response); err != nil {
			t.Fatalf("decode CLI %s response %q: %v", operation, output, err)
		}
		return response
	}
	client := gomcp.NewClient(&gomcp.Implementation{Name: "checkpoint-parity", Version: "v1alpha1"}, nil)
	session, err := client.Connect(ctx, &gomcp.CommandTransport{Command: exec.CommandContext(ctx, binary, "mcp", "--library", mcpRoot), TerminateDuration: 250 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("connect MCP: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		if err := session.Close(); err != nil {
			t.Errorf("close MCP: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- session.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("wait MCP: %v", err)
			}
		case <-cleanupCtx.Done():
			t.Errorf("MCP Close/Wait exceeded deadline: %v", cleanupCtx.Err())
		}
	})
	mcpCall := func(id, operation string, scope, input map[string]any) map[string]any {
		t.Helper()
		called, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: operation, Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": id, "scope": scope, "input": input}})
		if err != nil || called.IsError {
			t.Fatalf("MCP %s = %v/%t", operation, err, called != nil && called.IsError)
		}
		response, ok := called.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("MCP %s response = %T, want map", operation, called.StructuredContent)
		}
		return response
	}
	compare := func(id, operation string, scope, input map[string]any, want string) (map[string]any, map[string]any) {
		t.Helper()
		cli, mcp := cliCall(id, operation, scope, input), mcpCall(id, operation, scope, input)
		if got, expected := documentSemanticEnvelope(cli), documentSemanticEnvelope(mcp); !reflect.DeepEqual(got, expected) {
			t.Fatalf("%s parity mismatch\nCLI: %#v\nMCP: %#v", operation, got, expected)
		}
		for _, response := range []map[string]any{cli, mcp} {
			if got := response["outcome"].(map[string]any)["code"]; got != want {
				t.Fatalf("%s outcome = %q, want %q", operation, got, want)
			}
		}
		return cli, mcp
	}
	library := map[string]any{"kind": "library"}
	project := map[string]any{"kind": "project", "project_id": "project-a"}
	workstream := map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "workstream-a"}
	sessionScope := map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-a", "session_id": "session-a"}
	compare("checkpoint-project", "project.create", library, map[string]any{"project_id": "project-a", "name": "Alpha"}, app.OK)
	compare("checkpoint-workstream", "workstream.create", project, map[string]any{"workstream_id": "workstream-a", "origin": "parity"}, app.OK)
	compare("checkpoint-session", "session.open", workstream, map[string]any{"session_id": "session-a", "origin": "parity"}, app.OK)
	compare("checkpoint-source", "document.create", project, map[string]any{"document_id": "source-a", "revision_id": "source-r1", "expected_revision_id": nil, "content_base64": "AP9zb3VyY2UK", "provenance": map[string]any{"origin": "parity"}}, app.OK)
	save := map[string]any{"checkpoint_id": "checkpoint-a", "checkpoint_document_id": "checkpoint-document-a", "checkpoint_revision_id": "checkpoint-revision-a", "prose_base64": "AP9oYW5kb2ZmCg", "references": []map[string]any{{"document_id": "source-a", "revision_id": "source-r1"}}, "provenance": map[string]any{"origin": "parity", "client": "sdk", "model": "model-a"}}
	cliSaved, mcpSaved := compare("checkpoint-save", "checkpoint.save", sessionScope, save, app.OK)
	for _, response := range []map[string]any{cliSaved, mcpSaved} {
		result := response["result"].(map[string]any)
		fresh, present := result["references"].([]any)[0].(map[string]any)["fresh"]
		if result["prose_base64"] != "AP9oYW5kb2ZmCg" || !present || fresh != true || result["provenance"].(map[string]any)["model"] != "model-a" {
			t.Fatalf("checkpoint save result = %#v, want exact bytes, explicit fresh:true, and provenance", result)
		}
	}
	compare("checkpoint-read", "checkpoint.read", sessionScope, map[string]any{"checkpoint_id": "checkpoint-a"}, app.OK)
	compare("checkpoint-missing-session", "checkpoint.read", map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-a", "session_id": "session-missing"}, map[string]any{"checkpoint_id": "checkpoint-a"}, app.NotFound)
	unrelated := map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "workstream-unrelated"}
	compare("checkpoint-unrelated-workstream", "workstream.create", project, map[string]any{"workstream_id": "workstream-unrelated", "origin": "parity"}, app.OK)
	compare("checkpoint-unrelated-document", "document.create", unrelated, map[string]any{"document_id": "unrelated-document", "revision_id": "unrelated-r1", "expected_revision_id": nil, "content_base64": "dW5yZWxhdGVkCg", "provenance": map[string]any{"origin": "parity"}}, app.OK)
	freshCLI, freshMCP := compare("checkpoint-unrelated-read", "checkpoint.read", sessionScope, map[string]any{"checkpoint_id": "checkpoint-a"}, app.OK)
	for _, response := range []map[string]any{freshCLI, freshMCP} {
		fresh, present := response["result"].(map[string]any)["references"].([]any)[0].(map[string]any)["fresh"]
		if !present || fresh != true {
			t.Fatalf("unrelated workstream activity did not preserve explicit fresh:true: %#v", response)
		}
	}
	compare("checkpoint-source-advance", "document.update", project, map[string]any{"document_id": "source-a", "revision_id": "source-r2", "expected_revision_id": "source-r1", "content_base64": "dXBkYXRlZAo", "provenance": map[string]any{"origin": "parity"}}, app.OK)
	staleCLI, staleMCP := compare("checkpoint-stale-read", "checkpoint.read", sessionScope, map[string]any{"checkpoint_id": "checkpoint-a"}, app.OK)
	for _, response := range []map[string]any{staleCLI, staleMCP} {
		fresh, present := response["result"].(map[string]any)["references"].([]any)[0].(map[string]any)["fresh"]
		if !present || fresh != false {
			t.Fatalf("advanced referenced revision did not return explicit fresh:false: %#v", response)
		}
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close MCP before replay: %v", err)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("wait MCP before replay: %v", err)
	}
	session, err = client.Connect(ctx, &gomcp.CommandTransport{Command: exec.CommandContext(ctx, binary, "mcp", "--library", mcpRoot), TerminateDuration: 250 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("reconnect MCP before replay: %v", err)
	}
	replayedCLI, replayedMCP := compare("checkpoint-save", "checkpoint.save", sessionScope, save, app.OK)
	for index, response := range []map[string]any{replayedCLI, replayedMCP} {
		original := []map[string]any{cliSaved, mcpSaved}[index]["result"].(map[string]any)
		replayed := response["result"].(map[string]any)
		fresh, present := replayed["references"].([]any)[0].(map[string]any)["fresh"]
		if !present || fresh != false || replayed["checkpoint_id"] != original["checkpoint_id"] || replayed["checkpoint_document_id"] != original["checkpoint_document_id"] || replayed["checkpoint_revision_id"] != original["checkpoint_revision_id"] {
			t.Fatalf("replayed checkpoint = %#v, want unchanged identity and live explicit fresh:false", replayed)
		}
	}
	compare("checkpoint-save", "checkpoint.save", sessionScope, map[string]any{"checkpoint_id": "checkpoint-a", "checkpoint_document_id": "checkpoint-document-a", "checkpoint_revision_id": "checkpoint-revision-a", "prose_base64": "Y2hhbmdlZA", "references": []map[string]any{{"document_id": "source-a", "revision_id": "source-r1"}}, "provenance": map[string]any{"origin": "parity", "client": "sdk", "model": "model-a"}}, app.IdempotencyMismatch)
	compare("checkpoint-disconnect", "session.disconnect", sessionScope, map[string]any{"observed_by": "parity"}, app.OK)
	compare("checkpoint-disconnected-read", "checkpoint.read", sessionScope, map[string]any{"checkpoint_id": "checkpoint-a"}, app.Conflict)
	compare("checkpoint-resume", "session.resume", workstream, map[string]any{"session_id": "session-b", "source_session_id": "session-a", "origin": "parity"}, app.OK)
	resumed := map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-a", "session_id": "session-b"}
	compare("checkpoint-resumed-read", "checkpoint.read", resumed, map[string]any{"checkpoint_id": "checkpoint-a"}, app.OK)
	compare("checkpoint-fork", "workstream.fork", workstream, map[string]any{"workstream_id": "workstream-fork", "origin": "parity"}, app.OK)
	fork := map[string]any{"kind": "workstream", "project_id": "project-a", "workstream_id": "workstream-fork"}
	compare("checkpoint-fork-session", "session.open", fork, map[string]any{"session_id": "session-fork", "origin": "parity"}, app.OK)
	forkSession := map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-fork", "session_id": "session-fork"}
	compare("checkpoint-mismatched-binding", "checkpoint.read", map[string]any{"kind": "session", "project_id": "project-a", "workstream_id": "workstream-fork", "session_id": "session-a"}, map[string]any{"checkpoint_id": "checkpoint-a"}, app.BindingMismatch)
	compare("checkpoint-fork-read", "checkpoint.read", forkSession, map[string]any{"checkpoint_id": "checkpoint-a"}, app.ScopeDenied)
	large := base64.RawStdEncoding.EncodeToString(make([]byte, 64<<10))
	compare("checkpoint-limit", "checkpoint.save", resumed, map[string]any{"checkpoint_id": "checkpoint-limit", "checkpoint_document_id": "checkpoint-document-limit", "checkpoint_revision_id": "checkpoint-revision-limit", "prose_base64": large, "references": []map[string]any{{"document_id": "source-a", "revision_id": "source-r2"}}, "provenance": map[string]any{"origin": "parity"}}, app.OK)
	for _, root := range []string{cliRoot, mcpRoot} {
		path := filepath.Join(root, "projects", "project-a", "documents", "checkpoint-document-a", "revisions", "checkpoint-revision-a.md")
		if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
			t.Fatalf("tamper checkpoint backing prose: %v", err)
		}
	}
	compare("checkpoint-tampered-read", "checkpoint.read", resumed, map[string]any{"checkpoint_id": "checkpoint-a"}, app.IntegrityDiscrepancy)
}

func TestCommandTransportReapsUnresponsiveChild(t *testing.T) {
	if os.Getenv("MEMGRAPHAI_HANG_HELPER") == "1" {
		time.Sleep(time.Hour)
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandTransportReapsUnresponsiveChild$")
	cmd.Env = append(os.Environ(), "MEMGRAPHAI_HANG_HELPER=1")
	connection, err := (&gomcp.CommandTransport{Command: cmd, TerminateDuration: 20 * time.Millisecond}).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	_ = connection.Close()
	if cmd.ProcessState == nil {
		t.Fatal("child has no process state after bounded close; want reaped")
	}
}
