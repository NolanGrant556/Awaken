## ADDED Requirements

### Requirement: MCP Server Configuration Schema

Awaken SHALL accept MCP server configuration via JSON files at two scopes — project (`./awaken.mcp.json`) and user (`~/.awaken/mcp.json`) — and support an additional in-memory `dynamic` scope for runtime-injected servers (used by the connector layer).

Each server config MUST declare exactly one of five transport variants: `stdio`, `http` (streamable HTTP), `sse`, `ws`, `in-process`. Project scope overrides user scope when server names collide; `dynamic` scope is merged last.

The runtime MUST expand `${ENV_VAR}` placeholders in config string values. The runtime MUST validate the schema on load and log a warning for invalid entries WITHOUT aborting startup of valid entries.

#### Scenario: Loading a stdio server from project config

- **WHEN** `./awaken.mcp.json` contains a server entry `{ "type": "stdio", "command": "uvx", "args": ["mcp-server-time"] }`
- **THEN** the runtime registers the server under the configured name with scope `project` and transitions it to `pending` before attempting connection

#### Scenario: Project scope overriding user scope

- **WHEN** both project and user configs declare a server named `notion` with different transports
- **THEN** only the project config is used and a debug log records the override

#### Scenario: Environment variable expansion

- **WHEN** a config entry contains `"url": "https://${INTERNAL_HOST}/mcp"` and `INTERNAL_HOST=corp.example.com` is set
- **THEN** the runtime connects to `https://corp.example.com/mcp`

#### Scenario: Invalid config entry does not block startup

- **WHEN** one of three configured servers has a malformed transport field
- **THEN** the malformed entry is skipped with a warning log, the other two servers still start, and the malformed name is reported via `GET /v1/mcp/servers` with state `failed` and an error reason

### Requirement: Connection State Machine

Each MCP server MUST be represented by exactly one of five states at any time: `pending`, `connected`, `failed`, `needs-auth`, `disabled`. State transitions MUST follow the diagram:

- Initial → `pending`
- `pending` → `connected` on successful handshake
- `pending` → `failed` on transport error after retries exhausted
- `pending` → `needs-auth` when the server requires authentication that is not yet provided
- `connected` → `needs-auth` on 401 or session-expired response
- `connected` → `failed` on unrecoverable transport error
- any non-`disabled` state → `disabled` via explicit administrative action
- `disabled` → `pending` via explicit administrative action
- `failed` → `pending` via explicit reconnect action

The runtime MUST NOT lose configuration when transitioning to `failed` or `needs-auth`; the config is retained so the user can retry.

#### Scenario: Handshake succeeds

- **WHEN** the server responds to `initialize` within the connect timeout and capabilities are returned
- **THEN** the state transitions from `pending` to `connected` and the capabilities are stored in the server's state snapshot

#### Scenario: 401 on tool call switches to needs-auth

- **WHEN** a tool call against a `connected` HTTP server returns HTTP 401 or a JSON-RPC error indicating expired session
- **THEN** the state transitions to `needs-auth`, all tools from that server are unregistered from the tool pool, and the tool call returns a structured `needs_auth` error to the caller

#### Scenario: Reconnect attempts respect exponential backoff

- **WHEN** a `pending` server fails to connect
- **THEN** the runtime retries with delays of 1s, 2s, 4s, 8s, 16s, and after the fifth failure transitions to `failed`

#### Scenario: Disable removes tools but preserves config

- **WHEN** the operator disables a `connected` server via `POST /v1/mcp/servers/{name}/disable`
- **THEN** all tools from that server are unregistered, the state becomes `disabled`, and the original config remains so `enable` can restore it

### Requirement: Transport Support

The runtime SHALL support five transports with feature parity for the core protocol surface (`initialize`, `tools/list`, `tools/call`, `prompts/list`, `resources/list`, `notifications/tools/list_changed`):

- `stdio`: spawn a child process and communicate via stdin/stdout JSON-RPC framing.
- `http`: streamable HTTP per the MCP spec, including SSE upgrade for server-to-client notifications.
- `sse`: legacy SSE transport for servers that have not migrated to streamable HTTP.
- `ws`: WebSocket transport.
- `in-process`: in-Go channel-based transport for MCP servers implemented as local Go packages.

The runtime MUST NOT crash the host process if an in-process server panics; the panic MUST be recovered and the server transitioned to `failed` with the panic message in the error reason.

#### Scenario: stdio child process exit transitions server to failed

- **WHEN** a stdio MCP server's child process exits with non-zero status while in `connected` state
- **THEN** the state transitions to `failed`, the error reason includes the exit code, and the child process resources are released

#### Scenario: In-process server panic does not crash host

- **WHEN** an in-process MCP server handler panics during `tools/call`
- **THEN** the panic is recovered, the server transitions to `failed`, the panic message is stored in the error reason, and the host process continues serving other servers

### Requirement: Tool Pool Integration

When an MCP server transitions to `connected`, the runtime MUST register every tool from its `tools/list` response into the central tool registry (`src/tool/registry.go`) under the name `mcp__{serverName}__{toolName}`. When the server transitions away from `connected` (to any other state), the runtime MUST unregister all of that server's tools.

Each registered MCP tool MUST carry metadata describing its source and behavior, including at minimum: `source: "mcp"`, `serverName`, `toolName` (original unprefixed), `readOnly`, `destructive`, `openWorld`, `requiresAuth`. These fields are consumed by the downstream `tool-platform` capability for permission decisions.

The runtime MUST subscribe to `notifications/tools/list_changed` and refresh the tool registration for that server when received, without affecting other servers.

#### Scenario: Newly connected server registers its tools with prefixes

- **WHEN** server `slack` enters `connected` state with tools `[post_message, list_channels]`
- **THEN** the tool registry contains `mcp__slack__post_message` and `mcp__slack__list_channels`, each tagged with `source: "mcp"` and `serverName: "slack"`

#### Scenario: Disconnection unregisters tools

- **WHEN** server `slack` transitions from `connected` to `failed`
- **THEN** `mcp__slack__post_message` and `mcp__slack__list_channels` are removed from the registry and an agent attempting to call them receives a `tool_not_found` error

#### Scenario: list_changed notification refreshes only the affected server

- **WHEN** server `slack` sends a `notifications/tools/list_changed` notification with a new tool `react_message`
- **THEN** the registry adds `mcp__slack__react_message` and tools from other servers are unaffected

### Requirement: MCP Tool Policy Integration Uses No-Review Decisions

MCP tools MUST map into the no-review tool decision model: `allow`, `deny`, `transform`, `needs_auth`, `needs_runtime`, `unsupported`, or `needs_mass_message_review`. MCP runtime MUST NOT expose or depend on generic approval events, `ask` decisions, or approval resolve APIs for new runs.

Third-party MCP servers default to `fileBridgeScope: none`. They cannot read or write local files through Awaken unless a connector or explicit TaskScope file bridge grants a bounded path set. File bridge grants MUST be scoped to `TaskScope.readable_roots`, `TaskScope.writable_roots`, `TaskScope.output_roots`, and attached files.

When an MCP server or tool lacks authorization, the tool adapter returns `needs_auth`, server emits `AWAKEN_AUTH_REQUIRED`, and Desktop shows authorization confirmation/link. The runtime refreshes server state and the tool pool after authorization completes; it does not keep retrying the failed call as a hidden approval.

MCP tools that perform group/bulk outbound messaging MUST be tagged as `mass_message_send` and enter `AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED`. No real send API may be called before the dedicated mass-message review is approved.

#### Scenario: Third-party MCP has no file bridge by default

- **WHEN** a manually configured third-party MCP server connects
- **THEN** its tools register with `fileBridgeScope: none` unless an explicit bounded bridge is configured

#### Scenario: MCP needs auth becomes auth flow

- **WHEN** an MCP tool call returns 401 or the server is `needs-auth`
- **THEN** the run emits `AWAKEN_AUTH_REQUIRED`, the tool is not executed, and no generic approval prompt appears

#### Scenario: MCP mass message uses dedicated review

- **WHEN** an MCP tool is classified as `mass_message_send`
- **THEN** the runtime requests dedicated mass-message review and calls the real send tool only after this review is approved

### Requirement: Authentication Bootstrap

For HTTP and SSE transports, the runtime MUST support OAuth 2.0 authorization code flow with PKCE as the default authentication mechanism. The runtime MUST expose a `POST /v1/mcp/servers/{name}/auth` endpoint that returns an authorization URL for the UI to open; upon callback to the loopback redirect, the runtime exchanges the code for tokens and reconnects the server.

Tokens MUST be held via a `CredentialStore` interface so that a persistent implementation can be substituted later. The v1 default implementation MAY store tokens in process memory only; persistence is delivered by a downstream change.

The runtime MUST NOT silently swallow 401 errors on tool calls; the state machine transition to `needs-auth` (see Connection State Machine) is the authoritative response.

#### Scenario: Operator triggers OAuth for a needs-auth server

- **WHEN** the operator calls `POST /v1/mcp/servers/notion/auth` for a server in `needs-auth` state
- **THEN** the response body contains an authorization URL with PKCE code challenge and the runtime listens on a loopback port for the callback

#### Scenario: Callback exchanges code and reconnects

- **WHEN** the OAuth callback delivers a valid authorization code to the loopback redirect
- **THEN** the runtime exchanges the code for an access token, stores it via `CredentialStore`, transitions the server from `needs-auth` to `pending`, and the next handshake brings it to `connected`

### Requirement: Server HTTP API

The runtime SHALL expose four HTTP endpoints under `/v1/mcp/` that the desktop UI and downstream changes consume:

- `GET /v1/mcp/servers` — returns an array of `{ name, state, transport, scope, errorReason?, lastConnectedAt?, capabilities? }`.
- `POST /v1/mcp/servers/{name}/auth` — initiates authentication; returns authorization URL or a structured error.
- `POST /v1/mcp/servers/{name}/disable` and `POST /v1/mcp/servers/{name}/enable` — explicit administrative transitions.
- `GET /v1/mcp/tools` — returns the current tool pool including builtin and MCP-sourced tools, each with the full metadata block from the Tool Pool Integration requirement.

The endpoints MUST be registered in `src/server/router.go` next to the existing `/v1/runs/*` routes.

#### Scenario: GET /v1/mcp/servers returns all configured servers

- **WHEN** three servers are configured (two `connected`, one `needs-auth`) and an HTTP GET is issued to `/v1/mcp/servers`
- **THEN** the response is a 200 with a JSON array of three entries containing their states and metadata

#### Scenario: GET /v1/mcp/tools reflects current registration

- **WHEN** server `slack` has just been disabled
- **THEN** subsequent calls to `GET /v1/mcp/tools` do not include any `mcp__slack__*` tools while builtin tools and other MCP servers' tools remain

### Requirement: Dynamic Scope Injection API

The `mcp-runtime` Manager SHALL expose two methods that allow other Awaken subsystems (notably the `connectors` capability) to inject and remove MCP server configurations at runtime without editing on-disk config files:

- `InjectDynamic(sourceId string, serverName string, config McpServerConfig) error`
- `RemoveDynamic(sourceId string, serverName string) error`

Injected servers MUST be assigned `scope: dynamic` and MUST be removed when the source explicitly calls `RemoveDynamic`. Process restart MUST NOT persist dynamic-scope servers; the source is responsible for re-injecting on startup.

`sourceId` MUST be recorded on the server snapshot so HTTP API responses and logs can attribute the server to its source (e.g., `sourceConnectorId: "notion"`).

#### Scenario: Connector injects a notion server at runtime

- **WHEN** the connectors registry calls `Manager.InjectDynamic("notion", "notion", config)`
- **THEN** the runtime adds the server to the dynamic scope, attempts handshake, and exposes it in `GET /v1/mcp/servers` with `scope: "dynamic"` and `sourceConnectorId: "notion"`

#### Scenario: Removing a dynamic server unregisters tools

- **WHEN** the connectors registry calls `Manager.RemoveDynamic("notion", "notion")` while the server is `connected`
- **THEN** the runtime disconnects the server, unregisters all `mcp__notion__*` tools, and the snapshot no longer contains the entry

### Requirement: Source-Attributed Server Snapshot

The `GET /v1/mcp/servers` endpoint and the internal `Manager.Snapshot()` MUST include an optional `sourceConnectorId` field on each server entry when the server was injected via `InjectDynamic` by the connectors registry. Manual entries from `awaken.mcp.json` MUST NOT carry this field.

When the same server name is declared by both a manual config and a dynamic injection, the dynamic injection wins and the manual entry's snapshot MUST include `shadowedBy: "connector:<id>"`. Conversely, if priority rules dictate the manual wins (none defined in v1; reserved for future), the dynamic entry MUST include `shadowedBy: "manual"`.

#### Scenario: Manual server is shadowed by a connector with the same name

- **WHEN** `awaken.mcp.json` declares a server `notion` and the notion connector is connected (also producing a server named `notion`)
- **THEN** the snapshot contains two entries: the dynamic one is active with `sourceConnectorId: "notion"`, and the manual one is present with `shadowedBy: "connector:notion"` and is not registered into the tool pool
