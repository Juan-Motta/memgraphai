# Phase 1 experimental contract decisions
This P1.0 record fixes the contracts that P1.1 must test before implementing shared
behavior. It does not add source code, dependencies, tests, integrations, or a
compatibility promise.
## Decision status
| Topic | Status | Decision |
| --- | --- | --- |
| Local access | **User-confirmed** | CLI and MCP execute with the library owner's local permissions and an explicit, validated scope. |
| Authorization boundary | **User-confirmed** | Do not add per-agent grants, an authorization framework, or an OS sandbox. Scope validation prevents accidental cross-context operations; it is not a security boundary against an owner who can read the library. |
| Contract stability | **User-confirmed** | The CLI JSON and MCP contract is experimental and versioned. No stable-v1 compatibility promise exists. |
| Envelope and outcomes | **P1.0 decision** | Both adapters use the exact `memgraphai.experimental/v1alpha1` envelope and outcome codes below. |
| MCP implementation library | **Proposed; compatibility unverified** | P1.1/P1.2 should use the official Go MCP SDK, because it is the Go-native route to the MCP protocol while keeping application behavior transport-neutral. No module path, version, build result, or protocol compatibility is claimed until locally evidenced. |
## Access, bootstrap, and configuration
1. A request declares a scope; a path, current directory, environment value, session,
   connection, display name, or recency may suggest a candidate but may never supply
   omitted scope or authorization.
2. Existing-project access requires a valid explicit scope and successful owner-local
   filesystem access. There is no additional project-grant configuration or per-agent
   permission layer. Agent identity and path association never replace scope validation.
3. Library resolution is: an explicit adapter-supplied library root, then the owner's
   local library configuration, then the owner default `~/.memgraph/`. P1.1 must
   validate and normalize the selected root before opening it; it must not search
   parent repositories or silently switch roots.
4. Project discovery and project creation are bootstrap operations. They are allowed
   at the resolved owner-local library root without first requiring an existing
   project grant, so creation is not circular authorization. They still validate
   request shape and local filesystem/store failures and do not confer later
   cross-project access.
5. This posture does not claim containment from an owner-local process. It does not
   change the Phase 0 relation checks, integrity recovery, or explicit scope rules.
## Versioned transport-neutral envelope
Every CLI JSON request/result and every MCP tool request/result uses this semantic
shape; CLI human rendering is a presentation of the same result rather than a second
contract.
```json
{
  "contract_version": "memgraphai.experimental/v1alpha1",
  "operation_id": "opaque-nonempty-id",
  "operation": "operation-name",
  "scope": {"kind": "library|project|workstream|session", "project_id": "required-as-applicable", "workstream_id": "required-as-applicable", "session_id": "required-as-applicable"},
  "input": {},
  "page": {"limit": 50, "token": "optional-opaque-token"}
}
```

`contract_version`, `operation_id`, `operation`, and `scope` are required. `input`
is required and may be an empty object; `page` is allowed only for list/history
operations. An unsupported version returns `unsupported_contract_version` and does
not execute. Unknown fields are rejected as `invalid` in this alpha contract, rather
than silently ignored.
```json
{
  "contract_version": "memgraphai.experimental/v1alpha1",
  "operation_id": "opaque-nonempty-id",
  "outcome": {"code": "ok", "message": "stable non-content summary"},
  "result": {},
  "page": {"next_token": "optional-opaque-token"}
}
```

A response identifies this server's supported contract version and echoes a valid
operation ID; an absent or invalid ID is returned as `null` on validation errors.
Unsupported request versions still receive this server's versioned error envelope.
`result` is present
only for `ok`; `page.next_token` is present only when more results exist. Errors have
no partial `result`. `message` is a stable non-content summary, never a raw document,
query, path, title, or content-bearing storage error.
## Outcome taxonomy and noninteractive behavior
| Code | Domain meaning and adapter behavior |
| --- | --- |
| `ok` | The operation completed; any returned data is within the declared validated scope. |
| `invalid` | Required shape, IDs, bounds, or token format is invalid. |
| `not_found` | The explicit identity, revision, session, or resource is absent. |
| `ambiguous` | Resolution has multiple candidates; no candidate is selected. |
| `scope_denied` | The requested content falls outside the declared scope or owner-local filesystem access is denied; no per-agent grant lookup is involved. |
| `binding_mismatch` | Supplied project, workstream, session, or target relationships disagree. |
| `conflict` | An expected revision, lifecycle transition, or stale view cannot be applied. |
| `integrity_discrepancy` | Required targeted revision verification cannot safely serve the requested content. |
| `busy` / `retryable` | Bounded contention has not completed; neither exposes a raw storage error. |
| `unknown` | A durable outcome cannot yet be classified; it never asserts rollback. |
| `idempotency_mismatch` | An operation ID is reused with different immutable request identity. |
| `unsupported_contract_version` | The marker is not this experimental version. |
| `internal` | An unclassified implementation failure occurred without leaking internal detail. |

CLI JSON and MCP return the same `outcome.code`, scope decision, operation ID, and
bounded result semantics. CLI human output may be concise but must identify the same
code. Neither adapter prompts, opens a selector, retries interactively, broadens scope,
or translates missing, ambiguous, denied, conflict, or retryable conditions into
success.
## Pagination contract
Pagination applies only to explicitly paginable list and history operations. The
first page defaults to `limit: 50`; `1..200` is accepted, and any other limit is
`invalid`. Results use the operation's documented deterministic ordering and an
opaque cursor; callers must not construct or edit a token.

A v1alpha1 token is unpadded base64url canonical JSON with exactly `v`, `operation`,
`scope_digest`, `filter_digest`, `order`, `limit`, `cursor`, and `view_revision`.
`scope_digest` covers the complete declared scope, including applicable IDs;
`filter_digest` covers every selection input other than page; and `view_revision` is
the store-issued list/history view identity. The service, not either adapter,
recomputes and compares every binding before reading a continuation.

A malformed token, a token from another operation/scope/filter/order/limit, or an
unknown token version returns `invalid`. A well-formed token whose `view_revision`
is no longer current returns `conflict` with no results and requires a fresh first
page. Tokens never grant access, survive a contract-version change, select a default
scope, or promise that a mutable listing can be resumed after a changed view.
## MCP stdio and session interruption contract
MCP is a client-owned stdio process: it starts for the client connection, reads stdin,
writes protocol output on stdout, and exits when that connection ends. Phase 1 adds no
daemon, HTTP listener, IPC service, or connection-global current-session state. Each
MCP request carries its own full declared scope through the shared service.

| Event | Persistent session effect | Required result |
| --- | --- | --- |
| Explicit open | Creates a distinct `open` session after validated project/workstream binding. | Return the new session ID only within its validated scope. |
| Explicit close | Changes that exact `open` or `disconnected` session to terminal `closed`. | A later use as an active session is `conflict` or `not_found` according to the operation. |
| Observed attributable disconnect | Changes a non-closed session to nonterminal `disconnected`. | It is not close, resume, or permission to reuse context. |
| Unattributable transport loss | Makes no persisted lifecycle change. | The request is interrupted; no session is guessed or closed. |
| Explicit resume | Creates a new `open` session for an explicitly selected, valid workstream. | It references its requested handoff/session provenance but never reopens or inherits an arbitrary session. |
| Explicit fork | Creates a distinct workstream from an explicitly selected source. | It neither closes nor resumes the source; a later session open is explicit. |

An interleaved MCP connection may carry several logical session IDs, but a request
with a missing, closed, disconnected, or mismatched required binding is rejected
without using connection state. Disconnect reporting is an adapter observation, not
hidden context restoration, model-state restoration, or filesystem restoration.
## Future implementation responsibilities
| Area | P1.1/P1.2 responsibility |
| --- | --- |
| `internal/app/contract.go` | Parse and validate this envelope, normalize explicit scope, own outcome mapping, enforce limits/token bindings, and return transport-neutral data. It must not read adapter state or implement a new grant framework. |
| CLI adapter | Resolve local configuration, construct the envelope, render human or JSON output, measure final serialized bytes, and remain noninteractive. |
| MCP adapter | Own stdio lifecycle and SDK integration, construct the same envelope per request, serialize the same result, measure final protocol response bytes, and never retain routing authority in connection state. |
| Telemetry path | Attempt operation ID, timestamp, interface, declared scope, status, backend duration, serialized response bytes, and available counts; omit raw content and isolate recorder failure from the business response. |
## P1.1 behavioral RED handoff
P1.1 starts with failing behavioral tests, not prose-completeness tests:

1. A JSON CLI and MCP request with identical valid scope produce equivalent `ok`
   result/outcome semantics, while an unsupported version performs no operation.
2. Missing scope, ambiguous path resolution, denied explicit scope, mismatched binding,
   and stale expected revision return respectively `invalid`, `ambiguous`,
   `scope_denied`, `binding_mismatch`, and `conflict` through both adapters without a
   prompt or scope fallback.
3. Project creation at a valid resolved owner root succeeds without an existing project
   grant; a target outside the declared project is rejected, while another explicitly
   selected valid project remains accessible to the same local owner.
4. A continuation token reused with another scope/filter/limit is `invalid`; a validly
   bound token after its view changes is `conflict`; limits 0 and 201 are `invalid`.
5. Explicit close is terminal, observed disconnect is nonterminal, resume creates a
   new session only for the selected workstream, and interleaved MCP requests cannot
   borrow each other's scope.
6. A telemetry recorder failure leaves an otherwise successful response `ok`, while
   recorded serialized bytes equal the final JSON or MCP bytes and contain no prose.
## Boundary and delivery gate
No FTS retrieval, embeddings, automatic context assembly, hidden context restoration,
source implementation, test execution, dependency installation, or build claim is
made by this record. The official Go SDK choice remains compatibility-unverified until
future local evidence. P1.1–P1.6 source work is estimated at 1,220–2,120 lines for
Phase 1 as a whole, so the parent must obtain a human delivery decision under
`ask-on-risk` before authorizing a source slice that can exceed the 2,000-line budget.
