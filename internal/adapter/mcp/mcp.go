// Package mcp exposes P1.2 project operations through the official Go MCP SDK.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"memgraphai/internal/app"
	"memgraphai/internal/telemetry"
)

// ToolInput is the typed MCP tool shape; business validation remains in app.
type ToolInput struct {
	ContractVersion string         `json:"contract_version"`
	OperationID     string         `json:"operation_id"`
	Scope           app.Scope      `json:"scope"`
	Input           map[string]any `json:"input"`
	Page            *app.Page      `json:"page,omitempty"`
}

// NewServer constructs the exact project, continuity, and document operation tools.
func NewServer(projects app.ProjectService, continuity app.ContinuityService, documents app.DocumentService, recorder app.Service, checkpoints ...app.CheckpointService) *gomcp.Server {
	return newServer(projects, continuity, documents, recorder, nil, checkpoints...)
}

func newServer(projects app.ProjectService, continuity app.ContinuityService, documents app.DocumentService, recorder app.Service, wire *wireMetricWriter, checkpoints ...app.CheckpointService) *gomcp.Server {
	server := gomcp.NewServer(&gomcp.Implementation{Name: "memgraphai", Version: "v1alpha1"}, nil)
	for _, operation := range []string{
		"project.create", "project.list", "project.resolve",
		"project.association.add", "project.association.list", "project.association.remove",
	} {
		addTool(server, operation, projects.Handle, recorder, wire)
	}
	for _, operation := range []string{
		"workstream.create", "workstream.list", "workstream.fork", "session.open",
		"session.status", "session.close", "session.disconnect", "session.resume",
	} {
		addTool(server, operation, continuity.Handle, recorder, wire)
	}
	for _, operation := range []string{"document.create", "document.update", "document.list", "document.read", "document.history"} {
		addTool(server, operation, documents.Handle, recorder, wire)
	}
	if len(checkpoints) == 1 && checkpoints[0].Store != nil && checkpoints[0].Files != nil {
		for _, operation := range []string{"checkpoint.save", "checkpoint.read"} {
			addTool(server, operation, checkpoints[0].Handle, recorder, wire)
		}
	}
	return server
}

func addTool(server *gomcp.Server, operation string, handler app.Handler, executor app.Service, wire *wireMetricWriter) {
	gomcp.AddTool(server, &gomcp.Tool{Name: operation, Description: "MemGraph AI " + operation}, func(ctx context.Context, request *gomcp.CallToolRequest, input ToolInput) (*gomcp.CallToolResult, map[string]any, error) {
		businessInput, _ := json.Marshal(input.Input)
		raw, _ := json.Marshal(app.Request{ContractVersion: input.ContractVersion, OperationID: input.OperationID, Operation: operation, Scope: input.Scope, Input: businessInput, Page: input.Page})
		started := time.Now()
		recordedExecutor := executor
		if wire != nil {
			recordedExecutor.Recorder = nil
		}
		var result app.Result
		// Serialize business work with metric persistence, never with a pending response.
		// This avoids SQLite read-to-write upgrade contention from our own recorder.
		if wire != nil {
			wire.mu.Lock()
		}
		serialized, response := recordedExecutor.ExecuteJSON(ctx, raw, "mcp", func(ctx context.Context, request app.Request) app.Result {
			result = handler(ctx, request)
			return result
		})
		if wire != nil {
			wire.mu.Unlock()
		}

		var output map[string]any
		if err := json.Unmarshal(serialized, &output); err != nil {
			output = map[string]any{"outcome": map[string]string{"code": app.Internal}}
		}
		if wire != nil && response.OperationID != nil && request.Params != nil {
			wire.offer(ctx, requestIDFromMeta(request.Params.Meta), telemetry.Operation{OperationID: input.OperationID, RecordedAt: time.Now(), Interface: "mcp", ScopeKind: input.Scope.Kind, ProjectID: input.Scope.ProjectID, WorkstreamID: input.Scope.WorkstreamID, SessionID: input.Scope.SessionID, Outcome: response.Outcome.Code, BackendDuration: time.Since(started), ResultCount: result.ResultCount})
		}
		return nil, output, nil
	})
}

// RunStdio serves one client-owned MCP stdio connection until EOF.
func RunStdio(ctx context.Context, projects app.ProjectService, continuity app.ContinuityService, documents app.DocumentService, executor app.Service, checkpoints ...app.CheckpointService) error {
	return RunIO(ctx, projects, continuity, documents, executor, os.Stdin, os.Stdout, checkpoints...)
}

// RunIO permits tests and the executable to use the SDK's newline-delimited stdio framing.
func RunIO(ctx context.Context, projects app.ProjectService, continuity app.ContinuityService, documents app.DocumentService, executor app.Service, reader io.ReadCloser, writer io.WriteCloser, checkpoints ...app.CheckpointService) error {
	wire := &wireMetricWriter{WriteCloser: writer, recorder: executor.Recorder, pending: make(map[string]telemetry.Operation)}
	return newServer(projects, continuity, documents, executor, wire, checkpoints...).Run(ctx, &gomcp.IOTransport{Reader: newRequestIDReader(reader), Writer: wire})
}

type wireMetricWriter struct {
	io.WriteCloser
	recorder telemetry.Recorder
	mu       sync.Mutex
	pending  map[string]telemetry.Operation
	frame    []byte
}

// Offers are keyed by the connection's JSON-RPC request ID, never by the
// caller's reusable application operation ID or potentially identical payload.
func (w *wireMetricWriter) offer(ctx context.Context, requestID string, operation telemetry.Operation) {
	if w.recorder == nil || requestID == "" || operation.OperationID == "" || ctx.Err() != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending == nil || ctx.Err() != nil {
		return
	}
	// A completed request can still have a response buffered in a legacy SDK
	// batch. Retain its metric until the actual frame or connection close.
	w.pending[requestID] = operation
}

// Closing the underlying writer first unblocks any Write holding the mutex.
// Offers whose frames cannot be emitted are released without invented metrics.
func (w *wireMetricWriter) Close() error {
	err := w.WriteCloser.Close()
	w.mu.Lock()
	w.pending = nil
	w.frame = nil
	w.mu.Unlock()
	return err
}

func (w *wireMetricWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.WriteCloser.Write(data)
	if n == 0 {
		return n, err
	}
	w.frame = append(w.frame, data[:n]...)
	for {
		end := 0
		for end < len(w.frame) && w.frame[end] != '\n' {
			end++
		}
		if end == len(w.frame) {
			break
		}
		w.recordFrame(w.frame[:end+1])
		w.frame = w.frame[end+1:]
	}
	return n, err
}

func (w *wireMetricWriter) recordFrame(frame []byte) {
	if bytes.HasPrefix(bytes.TrimSpace(frame), []byte{'['}) {
		var batch []json.RawMessage
		if json.Unmarshal(frame, &batch) != nil {
			return
		}
		for _, response := range batch {
			w.recordResponse(response, len(frame))
		}
		return
	}
	w.recordResponse(frame, len(frame))
}

func (w *wireMetricWriter) recordResponse(raw []byte, frameBytes int) {
	message, err := jsonrpc.DecodeMessage(raw)
	if err != nil {
		return
	}
	response, ok := message.(*jsonrpc.Response)
	if !ok {
		return
	}
	key := requestIDKey(response.ID)
	metric, found := w.pending[key]
	if !found {
		return
	}
	delete(w.pending, key)
	metric.ResponseBytes = frameBytes
	_ = w.recorder.Record(context.Background(), metric)
}

func requestIDKey(id jsonrpc.ID) string {
	encoded, _ := json.Marshal(id.Raw())
	return string(encoded)
}

func requestIDFromMeta(meta gomcp.Meta) string {
	id, _ := meta[requestIDMeta].(string)
	return id
}
