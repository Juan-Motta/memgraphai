package mcp

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"memgraphai/internal/app"
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
	if err != nil || len(tools.Tools) != 6 {
		t.Fatalf("ListTools() = %d, %v; want six tools", len(tools.Tools), err)
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

func TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame(t *testing.T) {
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
		serverDone <- RunIO(ctx, app.ProjectService{Store: store}, app.Service{Recorder: store}, serverIn, capture)
	}()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "wire-test", Version: "v1alpha1"}, nil)
	session, err := client.Connect(ctx, &gomcp.IOTransport{Reader: clientIn, Writer: clientOut}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	_, err = session.CallTool(ctx, &gomcp.CallToolParams{Name: "project.create", Arguments: map[string]any{
		"contract_version": app.ContractVersion, "operation_id": "wire-exact", "scope": map[string]any{"kind": "library"},
		"input": map[string]any{"project_id": "project-wire", "name": "Wire"},
	}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	frame := capture.frameContaining("wire-exact")
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
	var recorded int
	if err := db.QueryRowContext(ctx, "SELECT response_bytes FROM operation_metrics WHERE operation_id = 'wire-exact'").Scan(&recorded); err != nil {
		t.Fatalf("read metric: %v", err)
	}
	if recorded != len(frame) {
		t.Fatalf("response_bytes = %d, captured frame bytes = %d; want exact equality", recorded, len(frame))
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
		if bytes.Contains(frame, []byte(operationID)) {
			return append([]byte(nil), frame...)
		}
	}
	return nil
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
