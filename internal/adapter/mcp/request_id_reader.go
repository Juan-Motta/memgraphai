package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// This reserved key exists only between the input reader and SDK handler. Client
// values are overwritten; no application arguments or outgoing frames are changed.
const requestIDMeta = "memgraphai.internal/jsonrpc-request-id"

// Keep the SDK's connection intact: a Connection decorator would hide its private
// session-update hook and silently disable negotiated JSON-RPC batch validation.
type requestIDReader struct {
	io.ReadCloser
	reader     *bufio.Reader
	pending    []byte
	pendingErr error
}

func newRequestIDReader(reader io.ReadCloser) *requestIDReader {
	return &requestIDReader{ReadCloser: reader, reader: bufio.NewReader(reader)}
}

func (r *requestIDReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		line, err := r.reader.ReadBytes('\n')
		if len(line) == 0 {
			return 0, err
		}
		newline := bytes.HasSuffix(line, []byte{'\n'})
		r.pending = bindRequestIDs(bytes.TrimSuffix(line, []byte{'\n'}))
		if newline {
			r.pending = append(r.pending, '\n')
		}
		r.pendingErr = err
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	if len(r.pending) == 0 {
		err := r.pendingErr
		r.pendingErr = nil
		return n, err
	}
	return n, nil
}

// Only insert metadata. Framing validity, method validation, duplicate request
// IDs, cancellation, and whether batches are permitted remain owned by the SDK.
func bindRequestIDs(raw []byte) []byte {
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte{'['}) {
		var batch []json.RawMessage
		if json.Unmarshal(raw, &batch) != nil {
			return raw
		}
		for i, item := range batch {
			batch[i] = bindRequestID(item)
		}
		encoded, err := json.Marshal(batch)
		if err != nil {
			return raw
		}
		return encoded
	}
	return bindRequestID(raw)
}

func bindRequestID(raw []byte) []byte {
	message, err := jsonrpc.DecodeMessage(raw)
	request, ok := message.(*jsonrpc.Request)
	if err != nil || !ok || !request.IsCall() || request.Method != "tools/call" {
		return raw
	}
	var params map[string]json.RawMessage
	if json.Unmarshal(request.Params, &params) != nil || params == nil {
		return raw
	}
	meta := map[string]json.RawMessage{}
	if existing, ok := params["_meta"]; ok {
		if json.Unmarshal(existing, &meta) != nil {
			return raw
		}
	}
	if meta == nil {
		meta = map[string]json.RawMessage{}
	}
	meta[requestIDMeta], _ = json.Marshal(requestIDKey(request.ID))
	params["_meta"], _ = json.Marshal(meta)
	request.Params, _ = json.Marshal(params)
	encoded, err := jsonrpc.EncodeMessage(request)
	if err != nil {
		return raw
	}
	return encoded
}
