### Requirement: Unified ToolDefinition Schema

Every tool registered into the central tool registry (`src/tool/registry.go`), regardless of source (`builtin | mcp | connector | skill | plugin`), MUST provide a complete `ToolDefinition` object at registration time. The registry MUST refuse registration for any tool whose `ToolDefinition` is missing required fields and MUST log the rejection.

Required fields:

- `name` — globally unique within the registry; namespaced prefixes (`mcp__`, `skill__`) preserved.
- `displayName` — UI-friendly label.
- `source` — one of `builtin`, `mcp`, `connector`, `skill`, `plugin`.
- `description` — natural-language description used by the LLM.
- `inputSchema` — JSON Schema for the tool's input.
- `annotations.readOnly` — boolean, whether the tool only reads state.
- `annotations.destructive` — boolean, whether the tool may write, delete, or notify external systems.
- `annotations.openWorld` — boolean, whether the tool reaches external networks (vs. purely local).
- `annotations.requiresAuth` — boolean, whether the tool requires credentials currently held by some `connected` server/connector.

Optional fields: `annotations.serverName`, `annotations.toolName` (original unprefixed), `permissionPolicy` (`always_allow | always_ask | per_session | one_time`).

#### Scenario: Tool with missing required annotation is rejected

- **WHEN** a tool registers with `ToolDefinition` lacking the `annotations.destructive` field
- **THEN** the registry rejects the registration, logs the missing field, and the tool does not appear in `GET /v1/tools`

#### Scenario: GET /v1/tools returns full metadata

- **WHEN** the registry contains a mix of builtin, MCP, and Skill tools
- **THEN** `GET /v1/tools` returns every tool with its full `ToolDefinition` including the source and the four required annotations

### Requirement: Annotations-Based Permission Decisions

The permission decision function `Decide(ctx, def, input) PermissionDecision` MUST base its outcome solely on `ToolDefinition` annotations and user-configured overrides — never on hard-coded tool name matches. The output `PermissionDecision` MUST have one of three actions (`allow | ask | deny`) plus a human-readable reason.

Default rules (applied when no user override exists):

- `readOnly == true` regardless of `openWorld` → `allow`.
- `readOnly == false` AND `destructive == true` → `ask`.
- `readOnly == false` AND `destructive == false` → `ask` (conservative default for unknown side-effects).
- `requiresAuth == true` AND the associated server/connector is not in `connected` state → `deny` with reason `needs_auth` (the agent layer maps this to the needs_auth tool-progress stage; the tool is not invoked).

User overrides from `~/.awaken/permissions.json` MUST take precedence over default rules. Overrides may target a specific tool name (e.g., `mcp__notion__update_page: always_ask`) or a server prefix (e.g., `mcp__notion__*: always_ask`).

#### Scenario: Read-only tool is auto-allowed

- **WHEN** the agent invokes `read_file` with `annotations.readOnly: true, destructive: false`
- **THEN** `Decide` returns `allow` with reason `read_only_tool_allowed_by_default`, no approval is requested, and the tool executes

#### Scenario: Destructive tool requires approval

- **WHEN** the agent invokes `apply_patch` with `annotations.destructive: true`
- **THEN** `Decide` returns `ask` and an `AWAKEN_APPROVAL_REQUESTED` event is emitted carrying the full `ToolDefinition` plus `riskHints.affectedSystems`

#### Scenario: needs_auth tool is denied before invocation

- **WHEN** the agent invokes `mcp__notion__update_page` while server `notion` is in `needs-auth` state
- **THEN** `Decide` returns `deny` with reason `needs_auth`, the tool is NOT invoked, and the agent receives a `AWAKEN_TOOL_PROGRESS` event with `stage: needs_auth` instead of an error

#### Scenario: User override forces always_ask

- **WHEN** `~/.awaken/permissions.json` declares `web_search: always_ask` and the agent invokes `web_search`
- **THEN** `Decide` returns `ask` despite `web_search` being read-only, and an approval request is emitted

### Requirement: Unified Tool Progress Events

The runtime SHALL emit a single event type `AWAKEN_TOOL_PROGRESS` for the full lifecycle of every tool invocation. The event payload MUST include: `toolCallId`, `toolName`, `source`, `stage` (`started | progress | completed | failed | needs_auth`), and MAY include `message`, `percentage` (0–100), `error`, `resultSummary`.

The runtime MUST emit `started` before invoking the tool and exactly one terminal event (`completed | failed | needs_auth`) per invocation. Intermediate `progress` events are optional but, when present, MUST arrive in monotonic order.

The protocol mapper at `src/protocol/agui/mapper.go` MUST forward `AWAKEN_TOOL_PROGRESS` to AG-UI-compatible event types so clients consuming `/v1/runs/{id}/events` see the lifecycle.

#### Scenario: Successful tool invocation lifecycle

- **WHEN** the agent invokes `web_search` and it returns within timeout
- **THEN** the run event stream contains `AWAKEN_TOOL_PROGRESS{stage:started}` followed by `AWAKEN_TOOL_PROGRESS{stage:completed}` for that `toolCallId`

#### Scenario: Tool reports progress percentage

- **WHEN** an MCP tool sends `notifications/progress` with 25% then 75% during a long-running call
- **THEN** the event stream contains two `AWAKEN_TOOL_PROGRESS{stage:progress, percentage:25}` then `{stage:progress, percentage:75}` events, followed by a terminal `completed` or `failed` event

#### Scenario: needs_auth stage replaces failure

- **WHEN** the agent attempts to invoke an MCP tool whose server is `needs-auth`
- **THEN** the stream contains exactly `AWAKEN_TOOL_PROGRESS{stage:started}` (optional, runtime may skip if denied pre-start) and `AWAKEN_TOOL_PROGRESS{stage:needs_auth}`; NO `failed` event is emitted for the same `toolCallId`

### Requirement: Tool Audit Records

Every tool invocation that reaches the `Decide` function MUST produce a `ToolAuditRecord` regardless of outcome (including `deny`). The record MUST include at minimum: `toolCallId`, `runId`, `toolName`, `source`, `annotations`, `decision` (the `PermissionDecision` action), `decisionReason`, `approvalOrigin` (`local_ui | system_policy | auto_allow | skill_preapproved | null`), `startedAt`, `completedAt`, `outcome` (`success | failed | needs_auth | denied`), and optionally `resultSummary` and `errorReason`.

The runtime SHALL write each `ToolAuditRecord` to the run's event history AND deliver it to all configured `AuditSink` implementations. The default implementations are `NoopSink`, `InMemorySink` (for tests), and `FileSink` (writing `~/.awaken/audit/{YYYY-MM-DD}.jsonl`). Writes to sinks MUST be asynchronous; sink queue overflow MUST drop the oldest records with a warning rather than blocking the runtime.

`GET /v1/runs/{id}/audit` MUST return the chronologically ordered list of `ToolAuditRecord`s for the given run id.

#### Scenario: Denied tool still produces audit record

- **WHEN** `Decide` returns `deny` for an `mcp__notion__update_page` call due to `needs_auth`
- **THEN** the audit record has `decision: "deny"`, `decisionReason: "needs_auth"`, `outcome: "needs_auth"`, and is retrievable via `GET /v1/runs/{id}/audit`

#### Scenario: FileSink writes daily JSONL

- **WHEN** three tool invocations complete on 2026-07-01
- **THEN** `~/.awaken/audit/2026-07-01.jsonl` contains three lines, each a valid JSON `ToolAuditRecord`, in invocation order

#### Scenario: Sink queue overflow drops oldest

- **WHEN** the audit queue is full and a new record arrives
- **THEN** the oldest record is dropped, a single warning is logged, and the runtime continues without blocking

### Requirement: Approval Request Payload Schema

When `Decide` returns `ask`, the runtime SHALL emit an `AWAKEN_APPROVAL_REQUESTED` event whose payload contains: `approvalId`, `toolDefinition` (the full `ToolDefinition`), `input` (the tool's arguments as a JSON object), `riskHints` (`{ destructive, openWorld, requiresAuth, affectedSystems: [string] }`), and `allowedActions` (`["approve", "reject", "edit_plan", "request_more_info"]`).

`affectedSystems` MUST list the server names and/or connector ids the tool would touch, derived from `annotations.serverName` and the connector mapping; for builtin tools without external systems, it is an empty array.

This payload schema is the contract between the runtime and the desktop Approval Center; the desktop MUST render approvals without re-querying the tool registry.

#### Scenario: Approval payload is self-contained

- **WHEN** an `ask` decision fires for `mcp__notion__update_page` with input `{ pageId: "abc" }`
- **THEN** the `AWAKEN_APPROVAL_REQUESTED` payload contains the full ToolDefinition, the input arguments, `riskHints.affectedSystems: ["notion"]`, and all four `allowedActions`

#### Scenario: Builtin tool approval has empty affectedSystems

- **WHEN** an `ask` decision fires for `shell` with a non-allowlisted command
- **THEN** the payload includes `riskHints.affectedSystems: []` and the tool definition's source is `builtin`

### Requirement: Tools and Audit HTTP API

The runtime SHALL expose two HTTP endpoints:

- `GET /v1/tools` — returns the full current tool pool with every tool's `ToolDefinition`. Tools made invisible to the LLM (e.g., shadowed Skills, unavailable Skills) MAY be excluded from this endpoint at the implementation's discretion; only registered active tools are guaranteed.
- `GET /v1/runs/{id}/audit` — returns the chronological list of `ToolAuditRecord` entries for the given run.

These endpoints MUST be registered in `src/server/router.go`.

#### Scenario: GET /v1/tools omits unavailable Skills

- **WHEN** Skill `customer-followup-daily` is `unavailable` due to missing connector
- **THEN** `GET /v1/tools` response does not contain `skill__customer-followup-daily` (the tool was unregistered when the Skill became unavailable)

#### Scenario: GET /v1/runs/{id}/audit returns chronological records

- **WHEN** run `r1` invoked three tools in order
- **THEN** the audit endpoint returns those three records in invocation order with their `startedAt` timestamps strictly non-decreasing