// Package mcp exposes P1.2 project operations through the official Go MCP SDK.
package mcp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

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

// NewServer constructs the exact project and continuity operation tools.
func NewServer(projects app.ProjectService, continuity app.ContinuityService, recorder app.Service) *gomcp.Server {
	return newServer(projects, continuity, recorder, nil)
}

func newServer(projects app.ProjectService, continuity app.ContinuityService, recorder app.Service, wire *wireMetricWriter) *gomcp.Server {
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
	return server
}

func addTool(server *gomcp.Server, operation string, handler app.Handler, executor app.Service, wire *wireMetricWriter) {
	gomcp.AddTool(server, &gomcp.Tool{Name: operation, Description: "MemGraph AI " + operation}, func(ctx context.Context, _ *gomcp.CallToolRequest, input ToolInput) (*gomcp.CallToolResult, map[string]any, error) {
		if wire != nil {
			wire.waitForPending(ctx)
		}
		businessInput, _ := json.Marshal(input.Input)
		raw, _ := json.Marshal(app.Request{ContractVersion: input.ContractVersion, OperationID: input.OperationID, Operation: operation, Scope: input.Scope, Input: businessInput, Page: input.Page})
		started := time.Now()
		recordedExecutor := executor
		if wire != nil {
			recordedExecutor.Recorder = nil
		}
		var result app.Result
		serialized, response := recordedExecutor.ExecuteJSON(ctx, raw, "mcp", func(ctx context.Context, request app.Request) app.Result {
			result = handler(ctx, request)
			return result
		})
		if wire != nil {
			wire.offer(telemetry.Operation{OperationID: input.OperationID, RecordedAt: time.Now(), Interface: "mcp", ScopeKind: input.Scope.Kind, ProjectID: input.Scope.ProjectID, WorkstreamID: input.Scope.WorkstreamID, SessionID: input.Scope.SessionID, Outcome: response.Outcome.Code, BackendDuration: time.Since(started), ResultCount: result.ResultCount})
		}
		var output map[string]any
		if err := json.Unmarshal(serialized, &output); err != nil {
			output = map[string]any{"outcome": map[string]string{"code": app.Internal}}
		}
		return nil, output, nil
	})
}

// RunStdio serves one client-owned MCP stdio connection until EOF.
func RunStdio(ctx context.Context, projects app.ProjectService, continuity app.ContinuityService, executor app.Service) error {
	return RunIO(ctx, projects, continuity, executor, os.Stdin, os.Stdout)
}

// RunIO permits tests and the executable to use the SDK's newline-delimited stdio framing.
func RunIO(ctx context.Context, projects app.ProjectService, continuity app.ContinuityService, executor app.Service, reader io.ReadCloser, writer io.WriteCloser) error {
	wire := &wireMetricWriter{WriteCloser: writer, recorder: executor.Recorder, pending: make(map[string]pendingMetric)}
	return newServer(projects, continuity, executor, wire).Run(ctx, &gomcp.IOTransport{Reader: reader, Writer: wire})
}

type wireMetricWriter struct {
	io.WriteCloser
	recorder telemetry.Recorder
	mu       sync.Mutex
	pending  map[string]pendingMetric
	frame    []byte
}

type pendingMetric struct {
	operation telemetry.Operation
	done      chan struct{}
}

func (w *wireMetricWriter) offer(operation telemetry.Operation) {
	if w.recorder == nil || operation.OperationID == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending[operation.OperationID] = pendingMetric{operation: operation, done: make(chan struct{})}
}

func (w *wireMetricWriter) waitForPending(ctx context.Context) {
	w.mu.Lock()
	pending := make([]chan struct{}, 0, len(w.pending))
	for _, metric := range w.pending {
		pending = append(pending, metric.done)
	}
	w.mu.Unlock()
	for _, done := range pending {
		select {
		case <-done:
		case <-ctx.Done():
			return
		}
	}
}

func (w *wireMetricWriter) Write(data []byte) (int, error) {
	n, err := w.WriteCloser.Write(data)
	if n == 0 {
		return n, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
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
	var response any
	if json.Unmarshal(frame, &response) != nil {
		return
	}
	for _, operationID := range responseOperationIDs(response) {
		metric, found := w.pending[operationID]
		if !found {
			continue
		}
		delete(w.pending, operationID)
		metric.operation.ResponseBytes = len(frame)
		_ = w.recorder.Record(context.Background(), metric.operation)
		close(metric.done)
	}
}

func responseOperationIDs(value any) []string {
	var IDs []string
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if operationID, ok := typed["operation_id"].(string); ok {
				IDs = append(IDs, operationID)
			}
			for _, child := range typed {
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return IDs
}
