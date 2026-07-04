### Requirement: Bridge Subscription Lifecycle

The runtime SHALL run a `mcp-skill-bridge` service that subscribes to MCP server lifecycle events via `mcpclient.Manager.Subscribe`. On each `Connected`, `PromptsListChanged`, or `ResourcesListChanged` event for a server, the bridge MUST invoke `syncForServer(serverName)`, which is idempotent — it computes the current set of MCP-sourced Skills for that server and reconciles the SkillRegistry to match.

On each `Disconnected` or `Disabled` event for a server, the bridge MUST remove all Skills currently bound to that server from the SkillRegistry.

`syncForServer` calls MUST be debounced per server: multiple triggers within 300ms collapse into a single sync, to absorb churn from rapid `list_changed` storms.

#### Scenario: Bridge syncs prompts when server connects

- **WHEN** server `crm` transitions to `connected` and its `prompts/list` returns two prompts `client-followup` and `weekly-summary`
- **THEN** the SkillRegistry contains Skills `mcp__crm__client-followup` and `mcp__crm__weekly-summary`, each with `source: mcp` and `mcpBinding.serverName: "crm"`

#### Scenario: Bridge removes Skills when server disables

- **WHEN** server `crm` (previously `connected` with bridged Skills) is disabled via `POST /v1/mcp/servers/crm/disable`
- **THEN** all `mcp__crm__*` Skills are removed from the SkillRegistry within one event tick

#### Scenario: Debounced syncs

- **WHEN** server `crm` emits five `notifications/prompts/list_changed` within 200ms
- **THEN** the bridge calls `syncForServer("crm")` exactly once

### Requirement: Prompt to Skill Mapping

For each MCP prompt returned by `prompts/list`, the bridge SHALL create a Skill with:

- `name` = `mcp__{serverName}__{promptName}` (kebab-case enforced via normalization if the prompt name contains spaces or uppercase)
- `description` = prompt's `description` field (or empty string if absent)
- `whenToUse` = prompt's `description` (no separate field exists in MCP; fall back to description)
- `arguments` = mapping of prompt's `arguments` array (`{ name, description, required }`) to Skill arguments
- `allowedTools` = `[]` (MCP prompts are pure prompt templates; agent's full tool pool is used)
- `requiredMcpServers` = `[serverName]`
- `requiredConnectors` = `[sourceConnectorId]` IF the server was injected by a connector, else `[]`
- `mcpBinding` = `{ serverName, promptName }`
- `sourceOrigin` = `mcp`

The bridge MUST cap a single server's bridged Skills at 100 entries. Beyond that limit, the bridge MUST truncate and log a warning identifying the server.

#### Scenario: Prompt with arguments becomes Skill with input schema

- **WHEN** server `crm` returns a prompt `{ name: "client-followup", description: "Generate follow-up", arguments: [{ name: "client_id", required: true }] }`
- **THEN** the resulting Skill `mcp__crm__client-followup` exposes a tool input schema requiring `client_id`

#### Scenario: Hundredth-and-first prompt is truncated

- **WHEN** server `bigvendor` returns 150 prompts in `prompts/list`
- **THEN** the bridge registers exactly 100 Skills (the first 100 by server-returned order) and logs a warning naming `bigvendor` and the truncation

### Requirement: Resource-Sourced Skills

The bridge SHALL also examine `resources/list` for resources with `mimeType: "text/x-prompt"` and treat them as additional prompt templates, applying the same mapping rules as `prompts/list`. The Skill name in this case MUST be `mcp__{serverName}__{resourceSlug}` where `resourceSlug` is derived from the resource URI's last path segment.

Resources with other mime types MUST NOT be bridged into Skills. They remain available for future context-resource integration but are out of scope for this capability.

#### Scenario: Prompt-typed resource is bridged

- **WHEN** server `wiki` returns a resource `{ uri: "wiki://prompts/sop-runbook", mimeType: "text/x-prompt", name: "SOP Runbook" }`
- **THEN** a Skill `mcp__wiki__sop-runbook` is registered with mcpBinding indicating the resource origin

#### Scenario: Document resource is ignored

- **WHEN** server `wiki` returns a resource with `mimeType: "text/markdown"`
- **THEN** no Skill is created for that resource

### Requirement: Skill Execution Fetches Prompt Body

When executing a Skill with `source: mcp`, the SkillExecutor MUST call the source server's `prompts/get` (or `resources/read` for resource-typed prompts) to fetch the prompt body at execution time. Bodies MUST NOT be cached across executions in v1 — each execution refetches.

The fetch MUST respect a 60s timeout aligned with MCP tool-call timeouts. On timeout or transport error, the executor MUST return a structured `mcp_fetch_failed` error including server name and prompt name; the run MUST NOT crash.

#### Scenario: Successful fetch executes Skill

- **WHEN** the agent calls `skill__mcp__crm__client-followup` with valid args and the server returns the prompt body within 60s
- **THEN** the body (with arg substitution) is appended to the active run per the Skill's `context` mode and execution proceeds

#### Scenario: Fetch timeout returns structured error

- **WHEN** the server takes longer than 60s to respond to `prompts/get`
- **THEN** the tool call returns a `mcp_fetch_failed` error with server and prompt names, and the agent receives that error to decide retry or alternative