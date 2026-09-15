package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"memgraphai/internal/app"
	"memgraphai/internal/store/sqlite"
	"memgraphai/internal/telemetry"
	"sync"
	"testing"
	"time"
)

type metricCapture struct {
	mu      sync.Mutex
	records []telemetry.Operation
}

func (c *metricCapture) Record(_ context.Context, op telemetry.Operation) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, op)
	return nil
}

type bufferCloser struct{ bytes.Buffer }

func (*bufferCloser) Close() error { return nil }

func TestMCPWhitespaceOperationDoesNotBlockNextCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	store, err := sqlite.Open(ctx, t.TempDir()+"/db.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()
	t.Cleanup(func() { serverIn.Close(); clientOut.Close(); clientIn.Close(); serverOut.Close() })
	done := make(chan error, 1)
	go func() {
		done <- RunIO(ctx, app.ProjectService{Store: store}, app.ContinuityService{}, app.DocumentService{}, app.Service{Recorder: store}, serverIn, serverOut)
	}()
	session, err := gomcp.NewClient(&gomcp.Implementation{Name: "liveness", Version: "test"}, nil).Connect(ctx, &gomcp.IOTransport{Reader: clientIn, Writer: clientOut}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { session.Close(); <-done }()
	for i, id := range []string{"   ", "next-call"} {
		callCtx, stop := context.WithTimeout(ctx, time.Second)
		called, err := session.CallTool(callCtx, &gomcp.CallToolParams{Name: "project.list", Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": id, "scope": map[string]any{"kind": "library"}, "input": map[string]any{}}})
		stop()
		if err != nil {
			t.Fatalf("call %d (%q) did not complete: %v", i, id, err)
		}
		got := called.StructuredContent.(map[string]any)["outcome"].(map[string]any)["code"]
		want := app.OK
		if i == 0 {
			want = app.Invalid
		}
		if got != want {
			t.Fatalf("call %d outcome=%v, want %s", i, got, want)
		}
	}
}

func TestMCPRepeatedOperationIDsKeepBothWireMetrics(t *testing.T) {
	recorder := &metricCapture{}
	wire := &wireMetricWriter{WriteCloser: &bufferCloser{}, recorder: recorder, pending: make(map[string]telemetry.Operation)}
	wire.offer(t.Context(), "1", telemetry.Operation{OperationID: "same", Outcome: app.OK})
	wire.offer(t.Context(), "2", telemetry.Operation{OperationID: "same", Outcome: app.OK})
	frame := []byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"structuredContent\":{\"operation_id\":\"same\"}}}\n")
	for i := 0; i < 2; i++ {
		if i == 1 {
			frame = bytes.Replace(frame, []byte(`"id":1`), []byte(`"id":2`), 1)
		}
		if _, err := wire.Write(frame); err != nil {
			t.Fatal(err)
		}
	}
	if len(recorder.records) != 2 {
		t.Fatalf("recorded %d responses, want both responses for reused operation ID", len(recorder.records))
	}
	for _, record := range recorder.records {
		if record.ResponseBytes != len(frame) {
			t.Fatalf("recorded bytes=%d, want %d", record.ResponseBytes, len(frame))
		}
	}
}

func TestMCPOutOfOrderResponsesMatchRequestNotOperationID(t *testing.T) {
	recorder := &metricCapture{}
	wire := &wireMetricWriter{WriteCloser: &bufferCloser{}, recorder: recorder, pending: make(map[string]telemetry.Operation)}
	first := map[string]any{"operation_id": "same", "outcome": map[string]any{"code": "ok"}}
	second := map[string]any{"operation_id": "same", "outcome": map[string]any{"code": "invalid"}}
	wire.offer(t.Context(), "1", telemetry.Operation{OperationID: "same", Outcome: app.OK, ProjectID: "first"})
	wire.offer(t.Context(), "2", telemetry.Operation{OperationID: "same", Outcome: app.Invalid, ProjectID: "second"})
	for i, envelope := range []map[string]any{second, first} {
		frame, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2 - i, "result": map[string]any{"structuredContent": envelope}})
		frame = append(frame, '\n')
		// Exercise partial writes as well as full frames.
		if _, err := wire.Write(frame[:10]); err != nil {
			t.Fatal(err)
		}
		if _, err := wire.Write(frame[10:]); err != nil {
			t.Fatal(err)
		}
		record := recorder.records[len(recorder.records)-1]
		if record.Outcome != envelope["outcome"].(map[string]any)["code"] || record.ResponseBytes != len(frame) {
			t.Fatalf("record=%+v for frame=%s", record, frame)
		}
	}
	if len(recorder.records) != 2 || recorder.records[0].ProjectID != "second" || recorder.records[1].ProjectID != "first" {
		t.Fatalf("out-of-order records=%+v", recorder.records)
	}
}

func TestMCPQueuedOffersSurviveRequestCompletionUntilConnectionClose(t *testing.T) {
	recorder := &metricCapture{}
	wire := &wireMetricWriter{WriteCloser: &bufferCloser{}, recorder: recorder, pending: make(map[string]telemetry.Operation)}
	ctx, cancel := context.WithCancel(t.Context())
	wire.offer(ctx, "1", telemetry.Operation{OperationID: "queued"})
	cancel()
	if len(wire.pending) != 1 {
		t.Fatal("completed request lost its queued response metric")
	}
	wire.offer(ctx, "2", telemetry.Operation{OperationID: "canceled-before-offer"})
	if len(wire.pending) != 1 {
		t.Fatal("canceled request created an offer")
	}
	if err := wire.Close(); err != nil {
		t.Fatal(err)
	}
	wire.offer(t.Context(), "3", telemetry.Operation{OperationID: "closed-connection"})
	if len(wire.pending) != 0 || len(recorder.records) != 0 {
		t.Fatal("connection close retained offers or invented metrics")
	}
}

func TestMCPIdenticalEnvelopesKeepRequestSpecificMetrics(t *testing.T) {
	recorder := &metricCapture{}
	wire := &wireMetricWriter{WriteCloser: &bufferCloser{}, recorder: recorder, pending: make(map[string]telemetry.Operation)}
	envelope := map[string]any{"operation_id": "same", "outcome": map[string]any{"code": "invalid"}}
	wire.offer(t.Context(), "1", telemetry.Operation{OperationID: "same", Outcome: app.Invalid, ProjectID: "first"})
	wire.offer(t.Context(), "22", telemetry.Operation{OperationID: "same", Outcome: app.Invalid, ProjectID: "second"})
	for _, id := range []int{22, 1} {
		frame, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"structuredContent": envelope}})
		frame = append(frame, '\n')
		if _, err := wire.Write(frame); err != nil {
			t.Fatal(err)
		}
		got := recorder.records[len(recorder.records)-1]
		want := "first"
		if id == 22 {
			want = "second"
		}
		if got.ProjectID != want || got.ResponseBytes != len(frame) {
			t.Fatalf("request %d metric=%+v, want project=%s bytes=%d", id, got, want, len(frame))
		}
	}
}

func TestMCPRuntimeOutOfOrderIdenticalRepliesKeepExactMetrics(t *testing.T) {
	firstReady, releaseFirst := make(chan struct{}), make(chan struct{})
	middleware := func(next gomcp.MethodHandler) gomcp.MethodHandler {
		return func(ctx context.Context, method string, request gomcp.Request) (gomcp.Result, error) {
			result, err := next(ctx, method, request)
			if method == "tools/call" {
				call := request.(*gomcp.CallToolRequest)
				var input ToolInput
				if json.Unmarshal(call.Params.Arguments, &input) == nil && input.Scope.ProjectID == "first" {
					close(firstReady)
					select {
					case <-releaseFirst:
					case <-ctx.Done():
					}
				}
			}
			return result, err
		}
	}
	session, recorder, capture, stop := metricRuntime(t, func(context.Context, app.Request) app.Result { return app.Result{Outcome: app.Invalid} }, middleware)
	firstDone := make(chan error, 1)
	call := func(project string) error {
		_, err := session.CallTool(t.Context(), &gomcp.CallToolParams{Name: "project.list", Arguments: map[string]any{"contract_version": app.ContractVersion, "operation_id": "same", "scope": map[string]any{"kind": "project", "project_id": project}, "input": map[string]any{}}})
		return err
	}
	go func() { firstDone <- call("first") }()
	select {
	case <-firstReady:
	case <-time.After(time.Second):
		t.Fatal("first reply did not reach hold point")
	}
	if err := call("second"); err != nil {
		t.Fatal(err)
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	stop()
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.records) != 2 || recorder.records[0].ProjectID != "second" || recorder.records[1].ProjectID != "first" {
		t.Fatalf("runtime metrics=%+v", recorder.records)
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	index := 0
	for _, frame := range bytes.SplitAfter(capture.buf.Bytes(), []byte{'\n'}) {
		var response any
		if json.Unmarshal(frame, &response) != nil || len(responseOperationIDs(response)) == 0 {
			continue
		}
		if recorder.records[index].ResponseBytes != len(frame) {
			t.Fatalf("metric %d bytes=%d want=%d", index, recorder.records[index].ResponseBytes, len(frame))
		}
		if bytes.Contains(frame, []byte(requestIDMeta)) {
			t.Fatal("internal correlation metadata leaked")
		}
		index++
	}
	if index != 2 {
		t.Fatalf("captured %d operation frames", index)
	}
}

func TestMCPRuntimeCancellationDoesNotBlockNextCall(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	session, _, _, stop := metricRuntime(t, func(ctx context.Context, request app.Request) app.Result {
		if request.OperationID == "cancel" {
			close(started)
			<-ctx.Done()
			close(canceled)
		}
		return app.Result{Outcome: app.OK}
	}, nil)
	defer stop()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	arguments := func(id string) map[string]any {
		return map[string]any{"contract_version": app.ContractVersion, "operation_id": id, "scope": map[string]any{"kind": "library"}, "input": map[string]any{}}
	}
	go func() {
		_, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "project.list", Arguments: arguments("cancel")})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled call=%v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("SDK did not deliver cancellation")
	}
	nextCtx, nextCancel := context.WithTimeout(t.Context(), time.Second)
	defer nextCancel()
	if _, err := session.CallTool(nextCtx, &gomcp.CallToolParams{Name: "project.list", Arguments: arguments("next")}); err != nil {
		t.Fatalf("next call=%v", err)
	}
}

func metricRuntime(t *testing.T, handler app.Handler, middleware gomcp.Middleware) (*gomcp.ClientSession, *metricCapture, *frameCapture, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	in, clientOut := io.Pipe()
	clientIn, out := io.Pipe()
	recorder := &metricCapture{}
	capture := &frameCapture{WriteCloser: out}
	wire := &wireMetricWriter{WriteCloser: capture, recorder: recorder, pending: make(map[string]telemetry.Operation)}
	server := gomcp.NewServer(&gomcp.Implementation{Name: "metric-runtime", Version: "test"}, nil)
	addTool(server, "project.list", handler, app.Service{}, wire)
	if middleware != nil {
		server.AddReceivingMiddleware(middleware)
	}
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx, &gomcp.IOTransport{Reader: newRequestIDReader(in), Writer: wire}) }()
	session, err := gomcp.NewClient(&gomcp.Implementation{Name: "metric-runtime-client", Version: "test"}, nil).Connect(ctx, &gomcp.IOTransport{Reader: clientIn, Writer: clientOut}, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			session.Close()
			cancel()
			in.Close()
			out.Close()
			clientOut.Close()
			clientIn.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("server did not stop")
			}
		})
	}
	t.Cleanup(stop)
	return session, recorder, capture, stop
}

func TestMCPLegacyBatchMetricsCountTheCompleteFrame(t *testing.T) {
	recorder := &metricCapture{}
	wire := &wireMetricWriter{WriteCloser: &bufferCloser{}, recorder: recorder, pending: make(map[string]telemetry.Operation)}
	wire.offer(t.Context(), "1", telemetry.Operation{OperationID: "first"})
	wire.offer(t.Context(), "2", telemetry.Operation{OperationID: "second"})
	frame := []byte("[{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}},{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{}}]\n")
	if _, err := wire.Write(frame); err != nil {
		t.Fatal(err)
	}
	if len(recorder.records) != 2 {
		t.Fatalf("records=%+v", recorder.records)
	}
	for _, record := range recorder.records {
		if record.ResponseBytes != len(frame) {
			t.Fatalf("bytes=%d want full batch frame=%d", record.ResponseBytes, len(frame))
		}
	}
}

func TestMCPRuntimeLegacyBatchRetainsMetricsUntilFinalFrame(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	in, clientOut := io.Pipe()
	clientIn, out := io.Pipe()
	t.Cleanup(func() { in.Close(); clientOut.Close(); clientIn.Close(); out.Close() })
	recorder := &metricCapture{}
	wire := &wireMetricWriter{WriteCloser: out, recorder: recorder, pending: make(map[string]telemetry.Operation)}
	firstCompleted, releaseLast := make(chan struct{}), make(chan struct{})
	server := gomcp.NewServer(&gomcp.Implementation{Name: "batch-metrics", Version: "test"}, nil)
	addTool(server, "project.list", func(context.Context, app.Request) app.Result { return app.Result{Outcome: app.Invalid} }, app.Service{}, wire)
	server.AddReceivingMiddleware(func(next gomcp.MethodHandler) gomcp.MethodHandler {
		return func(ctx context.Context, method string, request gomcp.Request) (gomcp.Result, error) {
			result, err := next(ctx, method, request)
			if method == "tools/call" {
				call := request.(*gomcp.CallToolRequest)
				if requestIDFromMeta(call.Params.Meta) == "1" {
					context.AfterFunc(ctx, func() { close(firstCompleted) })
				} else {
					select {
					case <-releaseLast:
					case <-ctx.Done():
					}
				}
			}
			return result, err
		}
	})
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx, &gomcp.IOTransport{Reader: newRequestIDReader(in), Writer: wire}) }()
	lines := make(chan []byte, 2)
	go func() {
		reader := bufio.NewReader(clientIn)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				select {
				case lines <- line:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	initialize := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"test"}}}` + "\n"
	if _, err := io.WriteString(clientOut, initialize); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lines:
	case <-ctx.Done():
		t.Fatal("initialization did not complete")
	}
	if _, err := io.WriteString(clientOut, "{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n"); err != nil {
		t.Fatal(err)
	}
	batch := []map[string]any{}
	for _, id := range []int{1, 2} {
		batch = append(batch, map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": "project.list", "arguments": map[string]any{"contract_version": app.ContractVersion, "operation_id": fmt.Sprintf("batch-%d", id), "scope": map[string]any{"kind": "library"}, "input": map[string]any{}}}})
	}
	raw, _ := json.Marshal(batch)
	if _, err := clientOut.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstCompleted:
	case <-ctx.Done():
		t.Fatal("first request did not complete while final response was held")
	}
	// Allow the request-completion cleanup callback to run before releasing the
	// final response. SDK success here means queued in a batch, not sent on wire.
	time.Sleep(20 * time.Millisecond)
	close(releaseLast)
	var frame []byte
	select {
	case frame = <-lines:
	case <-ctx.Done():
		t.Fatal("batch frame did not arrive")
	}
	clientOut.Close()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("server did not stop")
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.records) != 2 {
		t.Fatalf("recorded %d metrics for two successful SDK batch responses: %+v", len(recorder.records), recorder.records)
	}
	for _, record := range recorder.records {
		if record.ResponseBytes != len(frame) {
			t.Fatalf("%s bytes=%d, want whole frame=%d", record.OperationID, record.ResponseBytes, len(frame))
		}
	}
}
