package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRequestIDReaderPreservesNonCallsAndMalformedLines(t *testing.T) {
	for _, raw := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`,
		`{"jsonrpc":"2.0","id":1,"result":{}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"_meta":"invalid"}}`,
		`{"jsonrpc":`,
	} {
		t.Run(raw, func(t *testing.T) {
			input := raw + "\n"
			got, err := io.ReadAll(newRequestIDReader(io.NopCloser(strings.NewReader(input))))
			if err != nil || string(got) != input {
				t.Fatalf("read=%s/%v, want unchanged %s", got, err, input)
			}
		})
	}
}

func TestRequestIDReaderBindsActualIDAndPreservesArgumentsAndMetadata(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":"actual","method":"tools/call","params":{"name":"project.list","arguments":{"number":9007199254740993,"operation_id":"same"},"_meta":{"trace":"keep","memgraphai.internal/jsonrpc-request-id":"forged"}}}`
	reader := newRequestIDReader(io.NopCloser(strings.NewReader(raw)))
	var got bytes.Buffer
	buffer := make([]byte, 7)
	for {
		n, err := reader.Read(buffer)
		got.Write(buffer[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	message, err := jsonrpc.DecodeMessage(got.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var params struct {
		Meta      map[string]any  `json:"_meta"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(message.(*jsonrpc.Request).Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.Meta[requestIDMeta] != `"actual"` || params.Meta["trace"] != "keep" || !bytes.Contains(params.Arguments, []byte("9007199254740993")) {
		t.Fatalf("params=%+v", params)
	}
	if reader.pendingErr != nil || len(reader.pending) != 0 {
		t.Fatal("reader retained EOF data")
	}
}

func TestRequestIDReaderPreservesSDKBatchNegotiation(t *testing.T) {
	for _, version := range []string{"2025-03-26", "2025-06-18"} {
		t.Run(version, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			in, clientOut := io.Pipe()
			clientIn, out := io.Pipe()
			t.Cleanup(func() { in.Close(); clientOut.Close(); clientIn.Close(); out.Close() })
			done := make(chan error, 1)
			go func() {
				done <- gomcp.NewServer(&gomcp.Implementation{Name: "batch-test", Version: "test"}, nil).Run(ctx, &gomcp.IOTransport{Reader: newRequestIDReader(in), Writer: out})
			}()
			lines := make(chan []byte, 2)
			go func() {
				reader := bufio.NewReader(clientIn)
				for {
					line, err := reader.ReadBytes('\n')
					if len(line) > 0 {
						lines <- line
					}
					if err != nil {
						return
					}
				}
			}()
			initialize := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"` + version + `","capabilities":{},"clientInfo":{"name":"test","version":"test"}}}` + "\n"
			if _, err := io.WriteString(clientOut, initialize); err != nil {
				t.Fatal(err)
			}
			select {
			case <-lines:
			case <-ctx.Done():
				t.Fatal("initialize did not complete")
			}
			if _, err := io.WriteString(clientOut, "{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n"); err != nil {
				t.Fatal(err)
			}
			batch := "[{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"},{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"ping\"}]\n"
			if _, err := io.WriteString(clientOut, batch); err != nil {
				t.Fatal(err)
			}
			if version == "2025-03-26" {
				select {
				case line := <-lines:
					var responses []json.RawMessage
					if err := json.Unmarshal(line, &responses); err != nil || len(responses) != 2 {
						t.Fatalf("legacy batch=%s/%v", line, err)
					}
				case <-ctx.Done():
					t.Fatal("legacy batch did not complete")
				}
				clientOut.Close()
				select {
				case <-done:
				case <-ctx.Done():
					t.Fatal("server did not close")
				}
			} else {
				select {
				case err := <-done:
					if err == nil || !strings.Contains(err.Error(), "batching is not supported") {
						t.Fatalf("modern batch=%v", err)
					}
				case <-ctx.Done():
					t.Fatal("modern batch not rejected")
				}
			}
		})
	}
}

func TestRequestIDReaderKeepsBatchShapeAndBindsEachCall(t *testing.T) {
	raw := []byte(`[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"project.list","arguments":{}}},{"jsonrpc":"2.0","id":"two","method":"tools/call","params":{"name":"project.list","arguments":{},"_meta":null}}]`)
	var batch []json.RawMessage
	if err := json.Unmarshal(bindRequestIDs(raw), &batch); err != nil || len(batch) != 2 {
		t.Fatalf("batch=%s/%v", batch, err)
	}
	for i, item := range batch {
		msg, err := jsonrpc.DecodeMessage(item)
		if err != nil {
			t.Fatal(err)
		}
		var params gomcp.CallToolParamsRaw
		if err := json.Unmarshal(msg.(*jsonrpc.Request).Params, &params); err != nil {
			t.Fatal(err)
		}
		want := []string{"1", `"two"`}[i]
		if requestIDFromMeta(params.Meta) != want {
			t.Fatalf("meta=%v want=%s", params.Meta, want)
		}
	}
}
