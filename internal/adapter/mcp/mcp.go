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

// NewServer constructs the six exact project operation tools.
func NewServer(service app.ProjectService, recorder app.Service) *gomcp.Server {
	return newServer(service, recorder, nil)
}

func newServer(service app.ProjectService, recorder app.Service, wire *wireMetricWriter) *gomcp.Server {
	server := gomcp.NewServer(&gomcp.Implementation{Name: "memgraphai", Version: "v1alpha1"}, nil)
	for _, operation := range []string{
		"project.create", "project.list", "project.resolve",
		"project.association.add", "project.association.list", "project.association.remove",
	} {
		addTool(server, operation, service, recorder, wire)
	}
	return server
}

func addTool(server *gomcp.Server, operation string, service app.ProjectService, executor app.Service, wire *wireMetricWriter) {
	gomcp.AddTool(server, &gomcp.Tool{Name: operation, Description: "MemGraph AI " + operation}, func(ctx context.Context, _ *gomcp.CallToolRequest, input ToolInput) (*gomcp.CallToolResult, map[string]any, error) {
		businessInput, _ := json.Marshal(input.Input)
		raw, _ := json.Marshal(app.Request{ContractVersion: input.ContractVersion, OperationID: input.OperationID, Operation: operation, Scope: input.Scope, Input: businessInput, Page: input.Page})
		started := time.Now()
		recordedExecutor := executor
		if wire != nil {
			recordedExecutor.Recorder = nil
		}
		var result app.Result
		serialized, response := recordedExecutor.ExecuteJSON(ctx, raw, "mcp", func(ctx context.Context, request app.Request) app.Result {
			result = service.Handle(ctx, request)
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
func RunStdio(ctx context.Context, service app.ProjectService, executor app.Service) error {
	return RunIO(ctx, service, executor, os.Stdin, os.Stdout)
}

// RunIO permits tests and the executable to use the SDK's newline-delimited stdio framing.
func RunIO(ctx context.Context, service app.ProjectService, executor app.Service, reader io.ReadCloser, writer io.WriteCloser) error {
	wire := &wireMetricWriter{WriteCloser: writer, recorder: executor.Recorder, pending: make(map[string]telemetry.Operation)}
	return newServer(service, executor, wire).Run(ctx, &gomcp.IOTransport{Reader: reader, Writer: wire})
}

type wireMetricWriter struct {
	io.WriteCloser
	recorder telemetry.Recorder
	mu       sync.Mutex
	pending  map[string]telemetry.Operation
	frame    []byte
}

func (w *wireMetricWriter) offer(operation telemetry.Operation) {
	if w.recorder == nil || operation.OperationID == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending[operation.OperationID] = operation
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
		operation, found := w.pending[operationID]
		if !found {
			continue
		}
		delete(w.pending, operationID)
		operation.ResponseBytes = len(frame)
		_ = w.recorder.Record(context.Background(), operation)
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
