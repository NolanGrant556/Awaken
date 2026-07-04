## ADDED Requirements

### Requirement: MCP-Source Skill Metadata

`SkillDefinition` SHALL include two additional optional fields to represent MCP-sourced skills and MCP-dependent local skills:

- `mcpBinding`: present only when the Skill is bridged from an MCP server; contains at minimum `{ serverName: string, promptName: string, kind: "prompt" | "resource" }`. Absent for purely local Skills.
- `requiredMcpServers`: a string array of MCP server names whose `connected` state is required for this Skill to be `available`. May be present on bridged Skills (auto-derived) or on hand-authored local SKILL.md files (manually declared).

Skill availability MUST treat `requiredMcpServers` as an independent gate from `requiredConnectors`: a Skill is `available` only when both `requiredConnectors` AND `requiredMcpServers` are satisfied (every connector in `connected`, every server in `connected`).

The SKILL.md loader for `bundled`, `user`, and `project` sources MUST reject any frontmatter declaring a `name` starting with `mcp__`. The `mcp__` namespace is reserved for the bridge.

#### Scenario: Local SKILL.md may declare requiredMcpServers

- **WHEN** a project SKILL.md frontmatter includes `requiredMcpServers: [internal-search]`
- **THEN** the loaded Skill carries `requiredMcpServers: ["internal-search"]` and becomes `unavailable` whenever the `internal-search` MCP server is not `connected`

#### Scenario: Local SKILL.md with reserved prefix is rejected

- **WHEN** a user creates `~/.awaken/skills/mcp__foo__bar.md` or sets `name: mcp__foo__bar` in frontmatter
- **THEN** the loader skips the file with a warning explaining that the `mcp__` namespace is reserved

#### Scenario: Bridged Skill is unavailable when its source server disconnects

- **WHEN** the bridged Skill `mcp__crm__client-followup` has `requiredMcpServers: ["crm"]` and server `crm` transitions to `failed`
- **THEN** the Skill's state becomes `unavailable` with `unavailableReason` naming the `crm` server, and the corresponding LLM-facing tool is removed
