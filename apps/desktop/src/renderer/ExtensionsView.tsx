import { useEffect, useMemo, useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  Check,
  FileUp,
  FolderOpen,
  Plus,
  Plug,
  RefreshCw,
  Server,
  Settings,
  Trash2,
  Upload,
  Wrench,
  X,
} from "lucide-react";
import {
  AwakenButton as Button,
  AwakenDialog as Dialog,
} from "@awaken/design-system";
import type {
  RuntimeConnector,
  RuntimeLogEntry,
  RuntimeMcpServer,
  RuntimeSkill,
  RuntimeToolInfo,
} from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";

export type McpToolInfo = {
  name: string;
  description: string;
  readOnly: boolean;
};

export type ExtensionSection = "connectors" | "skills";

type ConnectorKind = "mcp" | "cli";
type SkillView = "project" | "user";
type SkillInstallScope = "project" | "global";

type SkillProjectGroup = {
  root: string;
  label: string;
  skills: RuntimeSkill[];
  errorMessage?: string;
};

type VisibleSkillProjectGroup = SkillProjectGroup & {
  visibleSkills: RuntimeSkill[];
};

type ExtensionsViewProps = {
  section: ExtensionSection;
  desktopReady: boolean;
  runtimeClient: RuntimeClient;
  onNotice: (message: string) => void;
};

const MCP_EXAMPLE = `{
  "mcpServers": {
    "example-server": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-server-example"
      ]
    }
  }
}`;

const CONNECTOR_KIND_TABS: Array<{ id: ConnectorKind; label: string }> = [
  { id: "mcp", label: "MCP" },
  { id: "cli", label: "CLI" },
];

const SKILL_VIEW_TABS: Array<{ id: SkillView; label: string }> = [
  { id: "project", label: "项目 Skills" },
  { id: "user", label: "全局 Skills" },
];

const CLI_CONNECTOR_KEYWORDS = ["cli", "dingtalk", "钉钉", "lark", "飞书"];

function connectorKind(connector: RuntimeConnector): ConnectorKind {
  const haystack = [
    connector.id,
    connector.displayName,
    connector.provider,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
  return CLI_CONNECTOR_KEYWORDS.some((keyword) => haystack.includes(keyword))
    ? "cli"
    : "mcp";
}

function connectorCategoryLabel(category: string): string {
  switch (category) {
    case "docs":
      return "文档";
    case "chat":
      return "沟通";
    case "code":
      return "开发";
    case "data":
      return "数据";
    default:
      return "其他";
  }
}

function stateLabel(state: string): string {
  switch (state) {
    case "connected":
      return "已连接";
    case "pending":
      return "连接中";
    case "needs-auth":
      return "需授权";
    case "failed":
      return "失败";
    case "available":
      return "可用";
    case "unavailable":
      return "不可用";
    case "shadowed":
      return "被覆盖";
    case "disabled":
      return "已停用";
    default:
      return "未连接";
  }
}

function sourceLabel(source: string): string {
  switch (source) {
    case "project":
      return "项目";
    case "user":
      return "用户";
    case "mcp":
      return "MCP";
    case "builtin":
      return "内置";
    case "local":
      return "本地";
    case "team":
      return "团队";
    default:
      return source;
  }
}

function skillSourceLabel(source: string): string {
  switch (source) {
    case "project":
      return "项目";
    case "user":
      return "全局";
    default:
      return sourceLabel(source);
  }
}

function matchesQuery(value: string | undefined, query: string): boolean {
  if (!query) return true;
  return (value ?? "").toLowerCase().includes(query);
}

function skillMatchesQuery(skill: RuntimeSkill, query: string): boolean {
  return (
    matchesQuery(skill.displayName, query) ||
    matchesQuery(skill.name, query) ||
    matchesQuery(skill.description, query) ||
    matchesQuery(skill.whenToUse, query) ||
    matchesQuery(skill.id, query)
  );
}

function projectLabelFromRoot(root: string): string {
  const cleaned = root.trim().replace(/[\\/]+$/, "");
  if (!cleaned) return "未命名项目";
  return cleaned.split(/[\\/]/).pop() || cleaned;
}

type SkillListLoadResult = {
  root?: string;
  skills: RuntimeSkill[];
  errorMessage?: string;
};

export async function loadSkillListsForRoots(
  roots: Array<string | undefined>,
  listSkills: RuntimeClient["listSkills"],
): Promise<SkillListLoadResult[]> {
  return Promise.all(
    roots.map(async (root) => {
      try {
        return {
          root,
          skills: await listSkills(root),
        };
      } catch (error) {
        return {
          root,
          skills: [],
          errorMessage:
            error instanceof Error ? error.message : "项目 Skills 加载失败",
        };
      }
    }),
  );
}

function dedupePaths(paths: Array<string | undefined>): string[] {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const path of paths) {
    const trimmed = path?.trim();
    if (!trimmed || seen.has(trimmed)) {
      continue;
    }
    seen.add(trimmed);
    result.push(trimmed);
  }
  return result;
}

function fileToBase64(file: File): Promise<string> {
  return file.arrayBuffer().then((buffer) => {
    const bytes = new Uint8Array(buffer);
    let binary = "";
    const chunkSize = 0x8000;
    for (let index = 0; index < bytes.length; index += chunkSize) {
      const chunk = bytes.subarray(index, index + chunkSize);
      binary += String.fromCharCode(...chunk);
    }
    return window.btoa(binary);
  });
}

export function ExtensionsView({
  section,
  desktopReady,
  runtimeClient,
  onNotice,
}: ExtensionsViewProps) {
  const [connectors, setConnectors] = useState<RuntimeConnector[]>([]);
  const [mcpServers, setMcpServers] = useState<RuntimeMcpServer[]>([]);
  const [skills, setSkills] = useState<RuntimeSkill[]>([]);
  const [skillProjectGroups, setSkillProjectGroups] = useState<SkillProjectGroup[]>([]);
  const [query, setQuery] = useState("");
  const [activeConnectorKind, setActiveConnectorKind] =
    useState<ConnectorKind>("mcp");
  const [activeSkillView, setActiveSkillView] = useState<SkillView>("project");
  const [expandedProjectRoots, setExpandedProjectRoots] = useState<Set<string>>(
    () => new Set(),
  );
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [busyConnectorId, setBusyConnectorId] = useState<string | null>(null);
  const [busyMcpServerName, setBusyMcpServerName] = useState<string | null>(null);
  const [mcpDialogOpen, setMcpDialogOpen] = useState(false);
  const [mcpConfigText, setMcpConfigText] = useState(MCP_EXAMPLE);
  const [configuringMcp, setConfiguringMcp] = useState(false);
  const [skillDialogOpen, setSkillDialogOpen] = useState(false);
  const [skillFile, setSkillFile] = useState<File | null>(null);
  const [skillDragActive, setSkillDragActive] = useState(false);
  const [installingSkill, setInstallingSkill] = useState(false);
  const [skillProjectRoot, setSkillProjectRoot] = useState("");
  const [skillInstallScope, setSkillInstallScope] =
    useState<SkillInstallScope>("project");
  const [skillInstallProjectRoot, setSkillInstallProjectRoot] = useState("");
  const [pickingSkillProjectRoot, setPickingSkillProjectRoot] = useState(false);
  const [mcpSettingsOpen, setMcpSettingsOpen] = useState(false);
  const [selectedMcpServer, setSelectedMcpServer] = useState<RuntimeMcpServer | null>(null);
  const [mcpSettingsConfigText, setMcpSettingsConfigText] = useState("");
  const [mcpSettingsScope, setMcpSettingsScope] = useState("");
  const [mcpTools, setMcpTools] = useState<McpToolInfo[]>([]);
  const [mcpLogs, setMcpLogs] = useState<RuntimeLogEntry[]>([]);
  const [savingMcpConfig, setSavingMcpConfig] = useState(false);
  const [loadingMcpSettings, setLoadingMcpSettings] = useState(false);

  const normalizedQuery = query.trim().toLowerCase();

  async function refresh(showLoading = true, projectRootOverride?: string): Promise<void> {
    if (!desktopReady) {
      setErrorMessage("Awaken runtime 未启动");
      return;
    }
    if (showLoading) {
      setLoading(true);
    }
    setErrorMessage(null);
    try {
      const runtimeInfo = await runtimeClient.getRuntimeInfo();
      const workspaceRegistry = await runtimeClient
        .readWorkspaceRegistry()
        .catch(() => ({
          lastWorkspacePath: undefined,
          registeredWorkspacePaths: [],
        }));
      const projectRoots = dedupePaths([
        projectRootOverride,
        skillProjectRoot,
        runtimeInfo?.defaultWorkspace,
        workspaceRegistry.lastWorkspacePath,
        ...workspaceRegistry.registeredWorkspacePaths,
      ]);
      const skillRoots = projectRoots.length > 0 ? projectRoots : [undefined];
      const [nextConnectors, nextMcpServers, nextSkillResults] = await Promise.all([
        runtimeClient.listConnectors(),
        runtimeClient.listMcpServers(),
        loadSkillListsForRoots(skillRoots, runtimeClient.listSkills),
      ]);
      const nextSkillLists = nextSkillResults.map((result) => result.skills);
      const nextProjectRoot =
        projectRootOverride?.trim() ||
        skillProjectRoot.trim() ||
        runtimeInfo?.defaultWorkspace ||
        projectRoots[0] ||
        "";
      if ((!skillProjectRoot.trim() || projectRootOverride) && nextProjectRoot) {
        setSkillProjectRoot(nextProjectRoot);
      }
      if ((!skillInstallProjectRoot.trim() || projectRootOverride) && nextProjectRoot) {
        setSkillInstallProjectRoot(nextProjectRoot);
      }
      setConnectors(nextConnectors);
      setMcpServers(nextMcpServers);
      setSkills(nextSkillLists[0] ?? []);
      setSkillProjectGroups(
        projectRoots.map((root, index) => ({
          root,
          label: projectLabelFromRoot(root),
          skills: (nextSkillLists[index] ?? []).filter(
            (skill) => skill.sourceOrigin === "project",
          ),
          errorMessage: nextSkillResults[index]?.errorMessage,
        })),
      );
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "扩展列表加载失败");
    } finally {
      if (showLoading) {
        setLoading(false);
      }
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [desktopReady]);

  useEffect(() => {
    if (!desktopReady || section !== "connectors") {
      return;
    }
    const timer = setInterval(() => {
      void refresh(false);
    }, 500);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [desktopReady, section]);

  useEffect(() => {
    if (!mcpSettingsOpen || !selectedMcpServer) {
      return;
    }
    const serverName = selectedMcpServer.name;
    const timer = setInterval(() => {
      void refreshMcpSettingsData(serverName);
    }, 500);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mcpSettingsOpen, selectedMcpServer?.name]);

  const visibleConnectors = useMemo(
    () =>
      connectors.filter(
        (connector) =>
          matchesQuery(connector.displayName, normalizedQuery) ||
          matchesQuery(connector.provider, normalizedQuery) ||
          matchesQuery(connector.id, normalizedQuery),
      ),
    [connectors, normalizedQuery],
  );

  const visibleMcpConnectors = useMemo(
    () =>
      visibleConnectors.filter(
        (connector) => connectorKind(connector) === "mcp",
      ),
    [visibleConnectors],
  );

  const visibleCliConnectors = useMemo(
    () =>
      visibleConnectors.filter(
        (connector) => connectorKind(connector) === "cli",
      ),
    [visibleConnectors],
  );

  const visibleAwakenBuiltInMcpConnectors = useMemo(
    () =>
      visibleMcpConnectors.filter(
        (connector) => connector.sourceOrigin === "builtin",
      ),
    [visibleMcpConnectors],
  );

  const visibleUserInstalledMcpConnectors = useMemo(
    () =>
      visibleMcpConnectors.filter(
        (connector) => connector.sourceOrigin !== "builtin",
      ),
    [visibleMcpConnectors],
  );

  const visibleMcpServers = useMemo(
    () =>
      mcpServers.filter(
        (server) =>
          matchesQuery(server.name, normalizedQuery) ||
          matchesQuery(server.transport, normalizedQuery) ||
          matchesQuery(server.errorReason, normalizedQuery),
      ),
    [mcpServers, normalizedQuery],
  );

  const visibleSkills = useMemo(
    () =>
      skills.filter((skill) => skillMatchesQuery(skill, normalizedQuery)),
    [skills, normalizedQuery],
  );

  const visibleProjectSkillGroups = useMemo<VisibleSkillProjectGroup[]>(
    () =>
      skillProjectGroups
        .map((group) => {
          const projectMatches =
            matchesQuery(group.label, normalizedQuery) ||
            matchesQuery(group.root, normalizedQuery);
          return {
            ...group,
            visibleSkills: projectMatches
              ? group.skills
              : group.skills.filter((skill) =>
                  skillMatchesQuery(skill, normalizedQuery),
                ),
          };
        })
        .filter(
          (group) =>
            !normalizedQuery ||
            matchesQuery(group.label, normalizedQuery) ||
            matchesQuery(group.root, normalizedQuery) ||
            group.visibleSkills.length > 0,
        ),
    [skillProjectGroups, normalizedQuery],
  );

  const visibleUserSkills = useMemo(
    () => visibleSkills.filter((skill) => skill.sourceOrigin === "user"),
    [visibleSkills],
  );

  const visibleMcpSkills = useMemo(
    () => visibleSkills.filter((skill) => skill.sourceOrigin === "mcp"),
    [visibleSkills],
  );

  async function handleConnectorAction(connector: RuntimeConnector): Promise<void> {
    setBusyConnectorId(connector.id);
    setErrorMessage(null);
    try {
      if (connector.state === "connected") {
        const result = await runtimeClient.disconnectConnector(connector.id);
        onNotice(result.message ?? `${connector.displayName} 已断开`);
      } else {
        const result = await runtimeClient.connectConnector(connector.id);
        onNotice(result.message ?? `${connector.displayName} 已连接`);
      }
      await refresh();
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "连接器操作失败");
    } finally {
      setBusyConnectorId(null);
    }
  }

  async function handleToggleMcpServer(server: RuntimeMcpServer): Promise<void> {
    setBusyMcpServerName(server.name);
    setErrorMessage(null);
    try {
      if (server.state === "disabled") {
        await runtimeClient.enableMcpServer(server.name);
        onNotice(`${server.name} 已启用`);
      } else {
        await runtimeClient.disableMcpServer(server.name);
        onNotice(`${server.name} 已停用`);
      }
      await refresh(false);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "MCP server 操作失败");
    } finally {
      setBusyMcpServerName(null);
    }
  }

  async function handleConfigureMcp(): Promise<void> {
    if (mcpConfigText.trim() === "") {
      setErrorMessage("请粘贴 MCP 配置 JSON");
      return;
    }
    setConfiguringMcp(true);
    setErrorMessage(null);
    try {
      const result = await runtimeClient.configureMcpServers({ configText: mcpConfigText });
      setMcpDialogOpen(false);
      onNotice(`已配置 ${result.serverNames.length} 个 MCP server`);
      await refresh();
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "MCP 配置失败");
    } finally {
      setConfiguringMcp(false);
    }
  }

  async function loadMcpSettings(server: RuntimeMcpServer): Promise<void> {
    setSelectedMcpServer(server);
    setMcpSettingsOpen(true);
    setLoadingMcpSettings(true);
    setErrorMessage(null);
    try {
      const [config, tools, logs] = await Promise.all([
        runtimeClient.getMcpServerConfig(server.name),
        runtimeClient.listMcpTools(),
        runtimeClient.getRuntimeLogs(),
      ]);
      setMcpSettingsConfigText(config.configText);
      setMcpSettingsScope(config.scope);
      setMcpTools(extractServerTools(tools, server.name));
      setMcpLogs(filterMcpLogs(logs, server.name));
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "加载 MCP 设置失败");
    } finally {
      setLoadingMcpSettings(false);
    }
  }

  async function refreshMcpSettingsData(serverName: string): Promise<void> {
    try {
      const [tools, logs] = await Promise.all([
        runtimeClient.listMcpTools(),
        runtimeClient.getRuntimeLogs(),
      ]);
      setMcpTools(extractServerTools(tools, serverName));
      setMcpLogs(filterMcpLogs(logs, serverName));
    } catch {
      // Ignore silent refresh errors to avoid overwriting user edits or
      // flashing messages while the dialog stays open.
    }
  }

  function extractServerTools(tools: RuntimeToolInfo[], serverName: string): McpToolInfo[] {
    const prefix = `mcp__${serverName}__`;
    return tools
      .filter((tool) => tool.source === "mcp" && tool.name.startsWith(prefix))
      .map((tool) => ({
        name: tool.name.slice(prefix.length),
        description: tool.description,
        readOnly: tool.readOnly,
      }));
  }

  async function handleSaveMcpConfig(): Promise<void> {
    if (!selectedMcpServer) return;
    if (mcpSettingsConfigText.trim() === "") {
      setErrorMessage("MCP 配置不能为空");
      return;
    }
    setSavingMcpConfig(true);
    setErrorMessage(null);
    try {
      await runtimeClient.updateMcpServerConfig(selectedMcpServer.name, {
        configText: mcpSettingsConfigText,
      });
      onNotice(`已更新 ${selectedMcpServer.name} 配置，正在重新连接`);
      const [config, logs] = await Promise.all([
        runtimeClient.getMcpServerConfig(selectedMcpServer.name),
        runtimeClient.getRuntimeLogs(),
      ]);
      setMcpSettingsConfigText(config.configText);
      setMcpLogs(filterMcpLogs(logs, selectedMcpServer.name));
      await refresh();
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "保存 MCP 配置失败");
    } finally {
      setSavingMcpConfig(false);
    }
  }

  async function handleDeleteMcpServer(): Promise<void> {
    if (!selectedMcpServer) return;
    if (!window.confirm(`确定要删除 MCP server "${selectedMcpServer.name}" 吗？`)) {
      return;
    }
    setSavingMcpConfig(true);
    setErrorMessage(null);
    try {
      await runtimeClient.deleteMcpServer(selectedMcpServer.name);
      onNotice(`已删除 ${selectedMcpServer.name}`);
      setMcpSettingsOpen(false);
      setSelectedMcpServer(null);
      await refresh();
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "删除 MCP server 失败");
    } finally {
      setSavingMcpConfig(false);
    }
  }

  function filterMcpLogs(logs: RuntimeLogEntry[], serverName: string): RuntimeLogEntry[] {
    const keywords = [serverName.toLowerCase(), "mcp"];
    return logs.filter((entry) =>
      keywords.some((keyword) => entry.line.toLowerCase().includes(keyword)),
    );
  }

  async function handleInstallSkill(): Promise<void> {
    if (!skillFile) {
      setErrorMessage("请选择 SKILL.md 文件或 skill 文件夹 zip");
      return;
    }
    const targetProjectRoot = skillInstallProjectRoot.trim();
    if (skillInstallScope === "project" && !targetProjectRoot) {
      setErrorMessage("请选择项目文件夹");
      return;
    }
    setInstallingSkill(true);
    setErrorMessage(null);
    try {
      const contentBase64 = await fileToBase64(skillFile);
      const result = await runtimeClient.installSkill({
        fileName: skillFile.name,
        contentBase64,
        scope: skillInstallScope,
        projectRoot: targetProjectRoot || undefined,
      });
      setSkillDialogOpen(false);
      setSkillFile(null);
      if (skillInstallScope === "project") {
        setSkillProjectRoot(targetProjectRoot);
      }
      onNotice(`已安装 ${result.skill.displayName || result.skill.name}`);
      await refresh(true, targetProjectRoot || undefined);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "Skill 安装失败");
    } finally {
      setInstallingSkill(false);
    }
  }

  async function handlePickSkillProjectRoot(): Promise<void> {
    setPickingSkillProjectRoot(true);
    setErrorMessage(null);
    try {
      const result = await runtimeClient.pickDirectory(
        skillInstallProjectRoot || skillProjectRoot || undefined,
      );
      if (result.canceled) {
        if (result.error) {
          setErrorMessage(result.error);
        }
        return;
      }
      setSkillInstallProjectRoot(result.path);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "项目文件夹选择失败");
    } finally {
      setPickingSkillProjectRoot(false);
    }
  }

  function toggleProjectSkillGroup(root: string): void {
    setExpandedProjectRoots((current) => {
      const next = new Set(current);
      if (next.has(root)) {
        next.delete(root);
      } else {
        next.add(root);
      }
      return next;
    });
  }

  function renderConnectorCard(connector: RuntimeConnector) {
    const connected = connector.state === "connected";
    return (
      <article className="extension-card" key={connector.id}>
        <div className="extension-card-icon" aria-hidden="true">
          <Plug size={20} strokeWidth={1.9} />
        </div>
        <div className="extension-card-copy">
          <div className="extension-card-title-row">
            <h3>{connector.displayName}</h3>
            <span className={`extension-state extension-state-${connector.state}`}>
              {stateLabel(connector.state)}
            </span>
          </div>
          <p>{connector.provider || connectorCategoryLabel(connector.category)}</p>
          <div className="extension-card-meta">
            <span>{connectorCategoryLabel(connector.category)}</span>
            <span>{sourceLabel(connector.sourceOrigin)}</span>
            {connector.serverNames?.length ? <span>{connector.serverNames.join(", ")}</span> : null}
          </div>
        </div>
        <button
          type="button"
          className="extension-card-action"
          aria-label={connected ? `断开 ${connector.displayName}` : `连接 ${connector.displayName}`}
          disabled={busyConnectorId === connector.id}
          onClick={() => {
            void handleConnectorAction(connector);
          }}
        >
          {connected ? <X size={18} strokeWidth={1.9} /> : <Plus size={18} strokeWidth={1.9} />}
        </button>
      </article>
    );
  }

  function renderMcpServerCard(server: RuntimeMcpServer) {
    const disabled = server.state === "disabled";
    return (
      <article className="extension-card extension-card-compact" key={server.name}>
        <div className="extension-card-icon" aria-hidden="true">
          <Server size={20} strokeWidth={1.9} />
        </div>
        <div className="extension-card-copy">
          <div className="extension-card-title-row">
            <h3>{server.name}</h3>
            <span className={`extension-state extension-state-${server.state}`}>
              {stateLabel(server.state)}
            </span>
          </div>
          <p>{server.errorReason || `${server.transport} · ${server.scope}`}</p>
        </div>
        <div className="extension-card-actions">
          <button
            type="button"
            className={`mcp-toggle-switch${disabled ? " mcp-toggle-switch-off" : " mcp-toggle-switch-on"}`}
            aria-label={disabled ? `启用 ${server.name}` : `停用 ${server.name}`}
            disabled={busyMcpServerName === server.name}
            onClick={() => {
              void handleToggleMcpServer(server);
            }}
          >
            <span className="mcp-toggle-switch-thumb" />
          </button>
          <button
            type="button"
            className="extension-card-action"
            aria-label={`设置 ${server.name}`}
            onClick={() => {
              void loadMcpSettings(server);
            }}
          >
            <Settings size={18} strokeWidth={1.9} />
          </button>
        </div>
      </article>
    );
  }

  function renderSkillCard(skill: RuntimeSkill) {
    return (
      <article className="extension-card" key={`${skill.id}:${skill.sourceOrigin}`}>
        <div className="extension-card-icon" aria-hidden="true">
          <Wrench size={20} strokeWidth={1.9} />
        </div>
        <div className="extension-card-copy">
          <div className="extension-card-title-row">
            <h3>{skill.displayName || skill.name}</h3>
            <span className={`extension-state extension-state-${skill.state}`}>
              {stateLabel(skill.state)}
            </span>
          </div>
          <p>{skill.whenToUse || skill.description}</p>
          <div className="extension-card-meta">
            <span>{skillSourceLabel(skill.sourceOrigin)}</span>
            <span>{skill.context}</span>
            {skill.requiredMcpServers?.length ? (
              <span>MCP: {skill.requiredMcpServers.join(", ")}</span>
            ) : null}
          </div>
        </div>
      </article>
    );
  }

  function renderProjectSkillGroup(group: VisibleSkillProjectGroup, index: number) {
    const expanded = expandedProjectRoots.has(group.root);
    const bodyId = `skill-project-group-${index}`;
    let countText = `${group.skills.length} Skill`;
    if (group.errorMessage) {
      countText = "不可用";
    } else if (
      normalizedQuery &&
      group.visibleSkills.length !== group.skills.length
    ) {
      countText = `${group.visibleSkills.length}/${group.skills.length} Skill`;
    }
    return (
      <section className="skill-project-group" key={group.root}>
        <button
          type="button"
          className="skill-project-toggle"
          aria-expanded={expanded}
          aria-controls={bodyId}
          onClick={() => toggleProjectSkillGroup(group.root)}
        >
          <span className="skill-project-chevron" aria-hidden="true">
            {expanded ? (
              <ChevronDown size={17} strokeWidth={1.9} />
            ) : (
              <ChevronRight size={17} strokeWidth={1.9} />
            )}
          </span>
          <span className="skill-project-copy">
            <span className="skill-project-name">{group.label}</span>
            <span className="skill-project-path" title={group.root}>
              {group.root}
            </span>
          </span>
          <span className="skill-project-count">{countText}</span>
        </button>
        {expanded ? (
          <div className="skill-project-body" id={bodyId}>
            {group.visibleSkills.length > 0 ? (
              <div className="extension-grid">
                {group.visibleSkills.map(renderSkillCard)}
              </div>
            ) : (
              <div className="extension-empty">
                {group.errorMessage ?? "暂无项目 Skills"}
              </div>
            )}
          </div>
        ) : null}
      </section>
    );
  }

  const title = section === "connectors" ? "连接器" : "技能";
  const subtitle =
    section === "connectors"
      ? "管理 MCP servers 与 CLI 连接器，连接外部系统和工具。"
      : "管理项目 Skills 与全局 Skills，扩展 Awaken 的执行能力。";
  const primaryAction =
    section === "connectors"
      ? { label: "配置 MCP", icon: <Plus size={16} strokeWidth={1.9} /> }
      : { label: "安装 Skill", icon: <Upload size={16} strokeWidth={1.9} /> };
  const showPrimaryAction =
    section === "skills" || activeConnectorKind === "mcp";

  return (
    <section className="extension-view" aria-label={title}>
      <div className="extension-view-inner">
        <header className="extension-header">
          <div>
            <h1>{title}</h1>
            <p>{subtitle}</p>
          </div>
          <div className="extension-header-actions">
            <button
              type="button"
              className="extension-icon-button"
              aria-label="刷新"
              disabled={loading}
              onClick={() => {
                void refresh();
              }}
            >
              <RefreshCw size={16} strokeWidth={1.9} />
            </button>
            {showPrimaryAction ? (
              <Button
                variant="primary"
                size="md"
                onClick={() => {
                  if (section === "connectors") {
                    setMcpDialogOpen(true);
                  } else {
                    setSkillInstallScope("project");
                    setSkillInstallProjectRoot(skillProjectRoot);
                    setSkillDialogOpen(true);
                  }
                }}
              >
                {primaryAction.icon}
                {primaryAction.label}
              </Button>
            ) : null}
          </div>
        </header>

        <div
          className={`extension-toolbar${
            section === "connectors" || section === "skills"
              ? " extension-toolbar-with-tabs"
              : ""
          }`}
        >
          {section === "connectors" || section === "skills" ? (
            <div
              className="extension-category-tabs"
              role="tablist"
              aria-label={section === "connectors" ? "连接器分类" : "技能视图"}
            >
              {(section === "connectors" ? CONNECTOR_KIND_TABS : SKILL_VIEW_TABS).map((tab) => (
                <button
                  key={tab.id}
                  type="button"
                  className={`extension-category-tab${
                    (section === "connectors"
                      ? activeConnectorKind === tab.id
                      : activeSkillView === tab.id)
                      ? " extension-category-tab-active"
                      : ""
                  }`}
                  role="tab"
                  aria-selected={
                    section === "connectors"
                      ? activeConnectorKind === tab.id
                      : activeSkillView === tab.id
                  }
                  onClick={() => {
                    if (section === "connectors") {
                      setActiveConnectorKind(tab.id as ConnectorKind);
                    } else {
                      setActiveSkillView(tab.id as SkillView);
                    }
                  }}
                >
                  {tab.label}
                </button>
              ))}
            </div>
          ) : null}
          <input
            value={query}
            placeholder="搜索..."
            onChange={(event) => setQuery(event.target.value)}
          />
        </div>

        {errorMessage ? <div className="extension-message" role="alert">{errorMessage}</div> : null}

        {section === "connectors" ? (
          <div className="extension-sections">
            {activeConnectorKind === "mcp" ? (
              <>
                <section className="extension-section">
                  <h2>Awaken 内置</h2>
                  <div className="extension-grid">
                    {visibleAwakenBuiltInMcpConnectors.length > 0 ? (
                      visibleAwakenBuiltInMcpConnectors.map(renderConnectorCard)
                    ) : (
                      <div className="extension-empty">暂无内置 MCP</div>
                    )}
                  </div>
                </section>
                <section className="extension-section">
                  <h2>用户安装</h2>
                  <div className="extension-grid">
                    {visibleUserInstalledMcpConnectors.length + visibleMcpServers.length > 0 ? (
                      <>
                        {visibleUserInstalledMcpConnectors.map(renderConnectorCard)}
                        {visibleMcpServers.map(renderMcpServerCard)}
                      </>
                    ) : (
                      <div className="extension-empty">暂无用户安装 MCP</div>
                    )}
                  </div>
                </section>
              </>
            ) : (
              <section className="extension-section" aria-label="CLI">
                <div className="extension-grid">
                  {visibleCliConnectors.length > 0 ? (
                    visibleCliConnectors.map(renderConnectorCard)
                  ) : (
                    <div className="extension-empty">暂无 CLI</div>
                  )}
                </div>
              </section>
            )}
          </div>
        ) : (
          <div className="extension-sections">
            {activeSkillView === "project" ? (
              <section className="extension-section" aria-label="项目 Skills">
                <div className="skill-project-list">
                  {visibleProjectSkillGroups.length > 0 ? (
                    visibleProjectSkillGroups.map(renderProjectSkillGroup)
                  ) : (
                    <div className="extension-empty">暂无项目 Skills</div>
                  )}
                </div>
              </section>
            ) : (
              <>
                <section className="extension-section" aria-label="全局 Skills">
                  <div className="extension-grid">
                    {visibleUserSkills.length > 0 ? (
                      visibleUserSkills.map(renderSkillCard)
                    ) : (
                      <div className="extension-empty">暂无全局 Skills</div>
                    )}
                  </div>
                </section>
                {visibleMcpSkills.length > 0 ? (
                  <section className="extension-section">
                    <h2>MCP 动态</h2>
                    <div className="extension-grid">
                      {visibleMcpSkills.map(renderSkillCard)}
                    </div>
                  </section>
                ) : null}
              </>
            )}
          </div>
        )}
      </div>

      <Dialog
        open={mcpDialogOpen}
        title="配置 MCP"
        onOpenChange={setMcpDialogOpen}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={configuringMcp}
              onClick={() => setMcpDialogOpen(false)}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={configuringMcp}
              disabled={mcpConfigText.trim() === ""}
              onClick={() => {
                void handleConfigureMcp();
              }}
            >
              <Check size={15} strokeWidth={1.9} />
              确认
            </Button>
          </>
        }
      >
        <div className="extension-dialog-body">
          <p>粘贴 MCP Servers 配置 JSON。支持 `mcpServers` 简写和 Awaken `servers` 配置。</p>
          <textarea
            className="extension-code-input"
            value={mcpConfigText}
            spellCheck={false}
            onChange={(event) => setMcpConfigText(event.target.value)}
          />
          <div className="extension-dialog-note">配置前确认来源和命令参数，避免运行不可信服务。</div>
        </div>
      </Dialog>

      <Dialog
        open={mcpSettingsOpen}
        title={selectedMcpServer ? `${selectedMcpServer.name} 设置` : "MCP 设置"}
        onOpenChange={(open) => {
          setMcpSettingsOpen(open);
          if (!open) {
            setSelectedMcpServer(null);
            setMcpSettingsConfigText("");
            setMcpSettingsScope("");
            setMcpTools([]);
            setMcpLogs([]);
          }
        }}
        footer={
          <div className="mcp-settings-footer">
            <Button
              variant="default"
              size="md"
              className="mcp-settings-delete-btn"
              style={{ color: "var(--aw-error)" }}
              disabled={savingMcpConfig}
              onClick={() => {
                void handleDeleteMcpServer();
              }}
            >
              <Trash2 size={15} strokeWidth={1.9} />
              删除
            </Button>
            <div className="mcp-settings-footer-actions">
              <Button
                variant="default"
                size="md"
                disabled={savingMcpConfig}
                onClick={() => setMcpSettingsOpen(false)}
              >
                取消
              </Button>
              <Button
                variant="primary"
                size="md"
                loading={savingMcpConfig}
                disabled={mcpSettingsConfigText.trim() === ""}
                onClick={() => {
                  void handleSaveMcpConfig();
                }}
              >
                <Check size={15} strokeWidth={1.9} />
                保存
              </Button>
            </div>
          </div>
        }
      >
        <div className="mcp-settings-dialog-body">
          {errorMessage ? <div className="extension-message" role="alert">{errorMessage}</div> : null}

          <section className="mcp-settings-section">
            <div className="mcp-settings-section-header">
              <h3>配置</h3>
              <span className="mcp-settings-scope">{mcpSettingsScope ? sourceLabel(mcpSettingsScope) : ""}</span>
            </div>
            <textarea
              className="mcp-settings-code"
              value={mcpSettingsConfigText}
              spellCheck={false}
              disabled={loadingMcpSettings}
              onChange={(event) => setMcpSettingsConfigText(event.target.value)}
            />
          </section>

          <section className="mcp-settings-section">
            <h3>方法</h3>
            {mcpTools.length > 0 ? (
              <ul className="mcp-tools-list">
                {mcpTools.map((tool) => (
                  <li key={tool.name}>
                    <code>{tool.name}</code>
                    <span>{tool.description}</span>
                    {tool.readOnly ? <span className="mcp-tool-readonly">只读</span> : null}
                  </li>
                ))}
              </ul>
            ) : (
              <div className="mcp-settings-empty">尚未获取到工具</div>
            )}
          </section>

          <section className="mcp-settings-section">
            <h3>日志</h3>
            {mcpLogs.length > 0 ? (
              <div className="mcp-logs-panel">
                {mcpLogs.map((entry) => (
                  <div key={entry.id} className={`mcp-log-line mcp-log-line-${entry.stream}`}>
                    <span className="mcp-log-time">{new Date(entry.timestamp).toLocaleTimeString()}</span>
                    <span className="mcp-log-text">{entry.line}</span>
                  </div>
                ))}
              </div>
            ) : (
              <div className="mcp-settings-empty">暂无相关日志</div>
            )}
          </section>
        </div>
      </Dialog>

      <Dialog
        open={skillDialogOpen}
        title="安装 Skill"
        onOpenChange={(open) => {
          setSkillDialogOpen(open);
          if (!open) {
            setSkillDragActive(false);
          }
        }}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={installingSkill}
              onClick={() => setSkillDialogOpen(false)}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={installingSkill}
              disabled={!skillFile}
              onClick={() => {
                void handleInstallSkill();
              }}
            >
              <Check size={15} strokeWidth={1.9} />
              安装
            </Button>
          </>
        }
      >
        <div className="extension-dialog-body">
          <div className="skill-install-scope">
            <span>安装范围</span>
            <div className="skill-install-segmented" role="tablist" aria-label="安装范围">
              <button
                type="button"
                className={skillInstallScope === "project" ? "skill-install-segment-active" : ""}
                aria-selected={skillInstallScope === "project"}
                onClick={() => setSkillInstallScope("project")}
              >
                项目 Skills
              </button>
              <button
                type="button"
                className={skillInstallScope === "global" ? "skill-install-segment-active" : ""}
                aria-selected={skillInstallScope === "global"}
                onClick={() => setSkillInstallScope("global")}
              >
                全局 Skills
              </button>
            </div>
          </div>
          {skillInstallScope === "project" ? (
            <div className="skill-install-project-root">
              <span>项目文件夹</span>
              <div className="skill-install-project-row">
                <code title={skillInstallProjectRoot}>
                  {skillInstallProjectRoot || "未选择"}
                </code>
                <button
                  type="button"
                  className="extension-icon-button"
                  aria-label="选择项目文件夹"
                  disabled={pickingSkillProjectRoot || installingSkill}
                  onClick={() => {
                    void handlePickSkillProjectRoot();
                  }}
                >
                  <FolderOpen size={16} strokeWidth={1.9} />
                </button>
              </div>
            </div>
          ) : null}
          <label
            className={`extension-dropzone${skillDragActive ? " extension-dropzone-active" : ""}`}
            onDragEnter={(event) => {
              event.preventDefault();
              setSkillDragActive(true);
            }}
            onDragOver={(event) => event.preventDefault()}
            onDragLeave={(event) => {
              event.preventDefault();
              setSkillDragActive(false);
            }}
            onDrop={(event) => {
              event.preventDefault();
              setSkillDragActive(false);
              setSkillFile(event.dataTransfer.files.item(0));
            }}
          >
            <input
              type="file"
              accept=".md,.zip"
              onChange={(event) => setSkillFile(event.target.files?.item(0) ?? null)}
            />
            <FileUp size={22} strokeWidth={1.9} />
            <strong>{skillFile ? skillFile.name : "拖放或点击上传"}</strong>
          </label>
          <p>支持 SKILL.md 文件，或包含 SKILL.md 的 skill 文件夹 zip。</p>
          <p className="extension-dialog-note">
            安装后请创建新会话或发起新任务使用；正在运行的会话不会中途加载新 Skill。
          </p>
        </div>
      </Dialog>
    </section>
  );
}
