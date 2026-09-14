// Package cli normalizes noninteractive project commands into the shared envelope.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"memgraphai/internal/app"
	"memgraphai/internal/revisionfs"
	"memgraphai/internal/store/sqlite"
	"memgraphai/internal/telemetry"
)

// Run executes one CLI operation and writes only the requested response to out.
func Run(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	library, jsonMode, operationID, rest, err := parseGlobal(args)
	if err != nil {
		fmt.Fprintln(errOut, "memgraphai:", err)
		return 2
	}
	root, err := ResolveLibrary(library)
	if err != nil {
		fmt.Fprintln(errOut, "memgraphai: library resolution failed")
		return 1
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		fmt.Fprintln(errOut, "memgraphai: library open failed")
		return 1
	}
	store, err := sqlite.Open(ctx, filepath.Join(root, "database.sqlite"))
	if err != nil {
		fmt.Fprintln(errOut, "memgraphai: library open failed")
		return 1
	}
	defer store.Close()

	var raw []byte
	if jsonMode && len(rest) == 0 {
		raw, err = io.ReadAll(in)
	} else {
		raw, err = commandRequest(operationID, rest)
	}
	if err != nil {
		fmt.Fprintln(errOut, "memgraphai: invalid command")
		return 2
	}
	projects := app.ProjectService{Store: store}
	continuity := app.ContinuityService{Store: store}
	documents := app.DocumentService{Store: store, Files: revisionfs.New(root, nil)}
	checkpoints := app.CheckpointService{Store: store, Files: revisionfs.New(root, nil)}
	handler := projects.Handle
	if request, code := app.ParseRequest(raw); code == "" {
		switch {
		case strings.HasPrefix(request.Operation, "workstream."), strings.HasPrefix(request.Operation, "session."):
			handler = continuity.Handle
		case strings.HasPrefix(request.Operation, "document."):
			handler = documents.Handle
		case strings.HasPrefix(request.Operation, "checkpoint."):
			handler = checkpoints.Handle
		}
	}
	started := time.Now()
	serialized, response := (app.Service{}).ExecuteJSON(ctx, raw, "cli", handler)
	written := 0
	if jsonMode {
		written, _ = out.Write(append(serialized, '\n'))
	} else {
		written, _ = fmt.Fprintln(out, response.Outcome.Code)
	}
	request, _ := app.ParseRequest(raw)
	_ = store.Record(ctx, telemetry.Operation{
		OperationID: request.OperationID, RecordedAt: time.Now(), Interface: "cli", ScopeKind: request.Scope.Kind,
		ProjectID: request.Scope.ProjectID, WorkstreamID: request.Scope.WorkstreamID, SessionID: request.Scope.SessionID,
		Outcome: response.Outcome.Code, BackendDuration: time.Since(started), ResponseBytes: written, ResultCount: response.MetricResultCount,
	})
	if response.Outcome.Code != app.OK {
		return 1
	}
	return 0
}

func parseGlobal(args []string) (library string, jsonMode bool, operationID string, rest []string, err error) {
	for len(args) > 0 {
		switch args[0] {
		case "--library", "--operation-id":
			if len(args) < 2 {
				return "", false, "", nil, errors.New("missing flag value")
			}
			if args[0] == "--library" {
				library = args[1]
			} else {
				operationID = args[1]
			}
			args = args[2:]
		case "--json":
			jsonMode = true
			args = args[1:]
		default:
			return library, jsonMode, operationID, args, nil
		}
	}
	return library, jsonMode, operationID, nil, nil
}

func commandRequest(operationID string, args []string) ([]byte, error) {
	if strings.TrimSpace(operationID) == "" || len(args) < 2 {
		return nil, errors.New("missing command")
	}
	operation, args, err := commandOperation(args)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(operation, "document.") {
		return documentCommandRequest(operationID, operation, args)
	}
	if strings.HasPrefix(operation, "checkpoint.") {
		return checkpointCommandRequest(operationID, operation, args)
	}
	scope := app.Scope{Kind: "library"}
	input := map[string]string{}
	var page *app.Page
	for len(args) > 0 {
		if len(args) < 2 || !strings.HasPrefix(args[0], "--") {
			return nil, errors.New("invalid command argument")
		}
		key, value := strings.TrimPrefix(args[0], "--"), args[1]
		if (operation == "project.list" || operation == "project.association.list" || operation == "workstream.list") && (key == "limit" || key == "token") {
			if page == nil {
				page = &app.Page{}
			}
			if key == "limit" {
				limit, err := strconv.Atoi(value)
				if err != nil {
					return nil, errors.New("invalid list limit")
				}
				page.Limit = &limit
			} else {
				page.Token = value
			}
		} else {
			setContinuityScope(&scope, operation, key, value, input)
		}
		args = args[2:]
	}
	setContinuityScopeKind(&scope, operation)
	inputJSON, _ := json.Marshal(input)
	return json.Marshal(app.Request{ContractVersion: app.ContractVersion, OperationID: operationID, Operation: operation, Scope: scope, Input: inputJSON, Page: page})
}

func commandOperation(args []string) (string, []string, error) {
	switch args[0] {
	case "project":
		if args[1] == "association" {
			if len(args) < 3 {
				return "", nil, errors.New("missing association command")
			}
			return "project.association." + args[2], args[3:], nil
		}
		return "project." + args[1], args[2:], nil
	case "workstream", "session", "document", "checkpoint":
		return args[0] + "." + args[1], args[2:], nil
	default:
		return "", nil, errors.New("unknown command")
	}
}

func documentCommandRequest(operationID, operation string, args []string) ([]byte, error) {
	scope := app.Scope{Kind: "project"}
	input := map[string]any{}
	provenance := map[string]string{}
	var page *app.Page
	for len(args) > 0 {
		if len(args) < 2 || !strings.HasPrefix(args[0], "--") {
			return nil, errors.New("invalid document argument")
		}
		key, value := strings.TrimPrefix(args[0], "--"), args[1]
		switch key {
		case "project-id":
			scope.ProjectID = value
		case "workstream-id":
			scope.Kind, scope.WorkstreamID = "workstream", value
		case "document-id", "revision-id", "content-base64":
			input[strings.ReplaceAll(key, "-", "_")] = value
		case "expected-revision-id":
			if value == "null" {
				input["expected_revision_id"] = nil
			} else {
				input["expected_revision_id"] = value
			}
		case "provenance-origin", "provenance-client", "provenance-model":
			provenance[strings.TrimPrefix(key, "provenance-")] = value
		case "limit":
			limit, err := strconv.Atoi(value)
			if err != nil {
				return nil, errors.New("invalid document limit")
			}
			page = &app.Page{Limit: &limit}
		case "token":
			if page == nil {
				page = &app.Page{}
			}
			page.Token = value
		default:
			return nil, errors.New("invalid document argument")
		}
		args = args[2:]
	}
	if len(provenance) > 0 {
		input["provenance"] = provenance
	}
	inputJSON, _ := json.Marshal(input)
	return json.Marshal(app.Request{ContractVersion: app.ContractVersion, OperationID: operationID, Operation: operation, Scope: scope, Input: inputJSON, Page: page})
}

func checkpointCommandRequest(operationID, operation string, args []string) ([]byte, error) {
	if operation != "checkpoint.save" && operation != "checkpoint.read" {
		return nil, errors.New("unknown checkpoint command")
	}
	scope := app.Scope{Kind: "session"}
	input := map[string]any{}
	provenance := map[string]string{}
	references := make([]map[string]string, 0)
	for len(args) > 0 {
		if len(args) < 2 || !strings.HasPrefix(args[0], "--") {
			return nil, errors.New("invalid checkpoint argument")
		}
		key, value := strings.TrimPrefix(args[0], "--"), args[1]
		switch key {
		case "project-id":
			scope.ProjectID = value
		case "workstream-id":
			scope.WorkstreamID = value
		case "session-id":
			scope.SessionID = value
		case "checkpoint-id", "checkpoint-document-id", "checkpoint-revision-id", "prose-base64":
			input[strings.ReplaceAll(key, "-", "_")] = value
		case "reference-document-id":
			references = append(references, map[string]string{"document_id": value})
		case "reference-revision-id":
			if len(references) == 0 || references[len(references)-1]["revision_id"] != "" {
				return nil, errors.New("reference revision must follow a document")
			}
			references[len(references)-1]["revision_id"] = value
		case "provenance-origin", "provenance-client", "provenance-model":
			provenance[strings.TrimPrefix(key, "provenance-")] = value
		default:
			return nil, errors.New("invalid checkpoint argument")
		}
		args = args[2:]
	}
	if operation == "checkpoint.save" {
		for _, reference := range references {
			if reference["revision_id"] == "" {
				return nil, errors.New("reference document requires a revision")
			}
		}
		input["references"] = references
		input["provenance"] = provenance
	} else if len(references) != 0 || len(provenance) != 0 {
		return nil, errors.New("checkpoint read has no references or provenance")
	}
	inputJSON, _ := json.Marshal(input)
	return json.Marshal(app.Request{ContractVersion: app.ContractVersion, OperationID: operationID, Operation: operation, Scope: scope, Input: inputJSON})
}

func setContinuityScope(scope *app.Scope, operation, key, value string, input map[string]string) {
	switch key {
	case "project-id":
		scope.ProjectID = value
	case "workstream-id":
		if operation == "workstream.fork" || strings.HasPrefix(operation, "session.") {
			scope.WorkstreamID = value
		} else {
			input["workstream_id"] = value
		}
	case "session-id":
		if operation == "session.status" || operation == "session.close" || operation == "session.disconnect" {
			scope.SessionID = value
		} else {
			input["session_id"] = value
		}
	case "new-workstream-id":
		input["workstream_id"] = value
	default:
		key = strings.ReplaceAll(key, "-", "_")
		if key == "id" {
			key = "project_id"
		}
		input[key] = value
	}
}

func setContinuityScopeKind(scope *app.Scope, operation string) {
	switch operation {
	case "workstream.create", "workstream.list", "project.association.add", "project.association.list", "project.association.remove":
		scope.Kind = "project"
	case "workstream.fork", "session.open", "session.resume":
		scope.Kind = "workstream"
	case "session.status", "session.close", "session.disconnect":
		scope.Kind = "session"
	}
}

// ResolveLibrary chooses explicit root, owner config, then the owner default.
func ResolveLibrary(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	defaultRoot := filepath.Join(home, ".memgraph")
	config, err := os.ReadFile(filepath.Join(defaultRoot, "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return defaultRoot, nil
	}
	if err != nil {
		return "", err
	}
	var decoded struct {
		LibraryRoot string `json:"library_root"`
	}
	if err := json.Unmarshal(config, &decoded); err != nil || decoded.LibraryRoot == "" {
		return "", errors.New("invalid library config")
	}
	return filepath.Abs(decoded.LibraryRoot)
}
