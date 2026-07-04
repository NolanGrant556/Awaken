## ADDED Requirements

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
