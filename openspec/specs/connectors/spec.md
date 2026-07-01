### Requirement: Connector Definition Schema

A connector SHALL be described by a `ConnectorDefinition` struct that includes at minimum: `id` (kebab-case unique), `displayName`, `category` (`docs | sheets | chat | crm | calendar | storage | database | custom`), `provider` (vendor name), `authType` (`oauth2 | apiKey | none`), `requiredScopes` (string array, may be empty), `trustLevel` (`builtin | team | local`), and `mcpConfigFactory` (a function returning one or more MCP server configs keyed by sub-name).

A single connector MAY produce multiple MCP server configs (for example, a vendor offering separate documents and search servers). Each produced config MUST be tagged with `sourceConnectorId` so that downstream `mcp-runtime` can attribute it.

#### Scenario: Builtin connector exposes 1:1 MCP server

- **WHEN** the builtin `notion` connector's `mcpConfigFactory` is invoked
- **THEN** it returns exactly one entry keyed by `notion` containing a valid streamable HTTP server config with `sourceConnectorId: "notion"`

#### Scenario: Builtin connector exposes multiple MCP servers

- **WHEN** a hypothetical `acme` connector's `mcpConfigFactory` returns two entries keyed `docs` and `search`
- **THEN** the runtime injects both into mcp-runtime under server names `acme/docs` and `acme/search`, each tagged with `sourceConnectorId: "acme"`

### Requirement: Connector Source Hierarchy

The connector registry SHALL load connectors from three sources, in descending priority: `builtin` (compiled into the binary), `team` (remote registry, fetched via the `TeamConnectorSource` interface), and `local` (user-edited `~/.awaken/connectors.json`).

When two sources declare a connector with the same `id`, the higher-priority source wins. The lower-priority entry MUST be retained in the registry as `shadowedBy: "<source>"` for UI visibility.

The `local` source schema MUST require each entry to provide `id`, `displayName`, `category`, and a valid MCP server config; OAuth fields are optional (defaulting to `authType: "none"`).

#### Scenario: Builtin overrides local with same id

- **WHEN** a local `connectors.json` declares an entry with `id: "notion"` and the builtin `notion` connector exists
- **THEN** only the builtin is active and the local entry appears in the registry snapshot with `shadowedBy: "builtin"`

#### Scenario: Local connector fills missing builtin

- **WHEN** the local config declares `id: "internal-wiki"` and no builtin or team source provides that id
- **THEN** the local connector is registered and its `mcpConfigFactory` is invoked when the user calls `connect`

### Requirement: Connector Lifecycle

A connector SHALL transition through states: `disconnected → pending → connected | needs-auth | failed`, with `disconnected` reachable from any state via an explicit disconnect.

`Connect(id)` MUST: (1) ensure required OAuth tokens are present via the shared `CredentialStore`, kicking off the OAuth flow if not; (2) invoke `mcpConfigFactory` to obtain MCP server configs; (3) inject them into the mcp-runtime `dynamic` scope via `Manager.InjectDynamic`. The connector's `connected` state MUST be derived from the corresponding mcp-runtime server states — not stored independently — so that the two layers cannot disagree.

`Disconnect(id)` MUST call `Manager.RemoveDynamic` for every server produced by this connector and best-effort revoke the OAuth token; the state transitions to `disconnected` even if revocation fails (a warning is logged).

#### Scenario: Connect triggers OAuth then injects servers

- **WHEN** the operator calls `POST /v1/connectors/notion/connect` and no token is cached
- **THEN** the response includes an authorization URL; once the OAuth callback succeeds, the runtime injects the notion MCP server into mcp-runtime, which transitions to `connected`, and the connector state derived view returns `connected`

#### Scenario: Disconnect removes servers and tools

- **WHEN** the operator calls `POST /v1/connectors/notion/disconnect`
- **THEN** all servers tagged `sourceConnectorId: "notion"` are removed from mcp-runtime, their tools are unregistered, and the connector state returns `disconnected`

#### Scenario: Underlying server failure surfaces on connector state

- **WHEN** a connected connector's underlying MCP server transitions to `failed`
- **THEN** the connector state derived view returns `failed` and the error reason from the server state is exposed to the connector snapshot

### Requirement: Connector HTTP API

The runtime SHALL expose three HTTP endpoints under `/v1/connectors/`:

- `GET /v1/connectors` — returns an array of `{ id, displayName, category, provider, trustLevel, state, errorReason?, lastConnectedAt?, shadowedBy? }`.
- `POST /v1/connectors/{id}/connect` — triggers the connect lifecycle; returns either an authorization URL (when OAuth is needed) or `{ state: "connected" }` when the connector is already authenticated.
- `POST /v1/connectors/{id}/disconnect` — triggers the disconnect lifecycle.

These endpoints MUST be registered in `src/server/router.go` alongside the existing `/v1/mcp/*` routes.

#### Scenario: GET /v1/connectors lists builtin and local entries

- **WHEN** two builtin connectors and one local connector are registered, the local one is shadowed
- **THEN** the response contains three entries, the shadowed local entry has `shadowedBy: "builtin"`, and all entries include their current derived state
