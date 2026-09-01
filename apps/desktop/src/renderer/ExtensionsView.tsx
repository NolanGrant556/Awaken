import { useEffect, useMemo, useRef, useState } from "react";
import {
  AwakenIcon,
  AwakenButton as Button,
  AwakenCliConnectorDetailDialog,
  AwakenConnectors,
  AwakenDialog as Dialog,
  AwakenFolder,
  AwakenMcpConfigDialog,
  AwakenMcpSettingsDialog,
  AwakenPluginInstallDialog,
  AwakenPlugins,
  AwakenProjectSkillsDialog,
  AwakenSkillCard,
  type AwakenCliConnectorAction,
  type AwakenConnectorCardItem,
  type AwakenMcpConfigMode,
  type AwakenPluginsView,
  type AwakenSkillItem,
  type AwakenSkillProject,
} from "@awaken/design-system";
import type {
  CliConnectorView,
  CliProviderId,
  RuntimeConnector,
  RuntimeLogEntry,
  RuntimeMcpServer,
  RuntimeMcpServerConfigResponse,
  RuntimePlugin,
  RuntimePluginComponent,
  RuntimePluginSource,
  RuntimePluginValidation,
  RuntimeProject,
  RuntimeSkill,
  RuntimeToolInfo,
} from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import {
  SkillPreviewContent,
  SkillPreviewDeleteButton,
  SkillPreviewDialog,
  type SkillPreviewPresentation,
  type SkillPreviewTarget,
} from "./SkillPreviewDialog";
import { cliConnectorProductCopy } from "./cliConnectorProduct";
import { PluginConsole } from "./plugins/PluginConsole";
import { pluginErrorCode, pluginErrorMessage } from "./plugins/pluginErrors";
import dingtalkLogo from "./assets/brands/dingtalk.png";
import feishuLogo from "./assets/brands/feishu.png";

export type McpToolInfo = {
  name: string;
  description: string;
  readOnly: boolean;
};

export type ExtensionSection = "connectors" | "skills" | "plugins";

type ConnectorKind = "mcp" | "cli";
export type SkillView = "awaken" | "project" | "user" | "agents";
type SkillInstallScope = "project" | "global";

function pluginValidationErrorMessage(error: unknown): string {
  switch (pluginErrorCode(error)) {
    case "PLUGIN_INVALID":
    case "PLUGIN_REQUEST_FAILED":
    case "PACKAGE_NO_USABLE_COMPONENTS":
      return "插件 ZIP 格式不符合要求";
    default:
      return pluginErrorMessage(error);
  }
}

type SkillProjectGroup = {
  id: string;
  root: string;
  label: string;
  folders: Array<{
    root: string;
    label: string;
    isPrimary: boolean;
  }>;
  skills: RuntimeSkill[];
  errorMessage?: string;
};

type VisibleSkillProjectGroup = SkillProjectGroup & {
  visibleSkills: RuntimeSkill[];
};

type VisibleAgentsSkill = {
  skill: RuntimeSkill;
  projectRoot?: string;
};

type PendingSkillDelete = {
  skill: RuntimeSkill;
  projectRoot?: string;
};

type ExtensionsViewProps = {
  section: ExtensionSection;
  desktopReady: boolean;
  runtimeClient: RuntimeClient;
  initialPlugins?: RuntimePlugin[];
  onNotice: (message: string) => void;
  activeWorkspaceRoot?: string;
  composerCatalogRevision?: number;
  navigationEpoch?: number;
  onNavigateSection?: (section: ExtensionSection) => void;
  onUsePlugin?: (plugin: RuntimePlugin) => void;
  onUsePluginComponent?: (
    plugin: RuntimePlugin,
    component: RuntimePluginComponent,
    commandArguments: string[],
  ) => void;
};

const NOOP_USE_PLUGIN = (_plugin: RuntimePlugin): void => undefined;
const NOOP_USE_PLUGIN_COMPONENT = (
  _plugin: RuntimePlugin,
  _component: RuntimePluginComponent,
  _commandArguments: string[],
): void => undefined;

const MCP_STDIO_EXAMPLE = `{
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

const MCP_STREAMABLE_HTTP_EXAMPLE = `{
  "mcpServers": {
    "remote-example": {
      "type": "streamable-http",
      "url": "https://mcp.example.test/mcp"
    }
  }
}`;

export function nextMcpConfigTextForMode(
  currentValue: string,
  nextMode: AwakenMcpConfigMode,
): string {
  const normalized = currentValue.trim();
  const isUntouchedExample =
    normalized === MCP_STDIO_EXAMPLE.trim() ||
    normalized === MCP_STREAMABLE_HTTP_EXAMPLE.trim();
  if (normalized !== "" && !isUntouchedExample) {
    return currentValue;
  }
  return nextMode === "stdio" ? MCP_STDIO_EXAMPLE : MCP_STREAMABLE_HTTP_EXAMPLE;
}

export function inferMcpConfigMode(value: string): AwakenMcpConfigMode | null {
  const normalized = value.trim();
  if (/^https?:\/\//i.test(normalized)) {
    return "streamable-http";
  }
  if (!normalized.startsWith("{")) {
    return null;
  }

  try {
    const payload = JSON.parse(normalized) as {
      servers?: Record<string, unknown>;
      mcpServers?: Record<string, unknown>;
    };
    const entries = [
      ...Object.values(payload.servers ?? {}),
      ...Object.values(payload.mcpServers ?? {}),
    ];
    let hasStdio = false;
    let hasRemote = false;
    for (const entry of entries) {
      if (!entry || typeof entry !== "object" || Array.isArray(entry)) continue;
      const config = entry as Record<string, unknown>;
      hasStdio ||= typeof config.command === "string" || config.stdio !== undefined;
      hasRemote ||= typeof config.url === "string" || config.http !== undefined || config.sse !== undefined;
    }
    if (hasRemote && !hasStdio) return "streamable-http";
    if (hasStdio && !hasRemote) return "stdio";
  } catch {
    return null;
  }
  return null;
}

export const DEFAULT_SKILL_VIEW: SkillView = "awaken";

export const SKILL_VIEW_TABS: Array<{ id: SkillView; label: string }> = [
  { id: "awaken", label: "Awaken Skills" },
  { id: "project", label: "项目 Skills" },
  { id: "user", label: "全局 Skills" },
  { id: "agents", label: "Agents Skills" },
];

export function connectorKind(connector: RuntimeConnector): ConnectorKind {
  return connector.kind;
}

export function isConnectorVisibleInCatalog(
  connector: Pick<RuntimeConnector, "catalogPlacement">,
): boolean {
  return connector.catalogPlacement !== "plugin_only";
}

export function connectorOwnedMcpServerNames(
  connectors: RuntimeConnector[],
): Set<string> {
  const names = new Set<string>();
  for (const connector of connectors) {
    if (connectorKind(connector) !== "mcp") continue;
    for (const name of connector.serverNames ?? []) {
      if (name.trim()) names.add(name);
    }
  }
  return names;
}

export function isPluginMcpServerName(name: string): boolean {
  return name.startsWith("plugin__");
}

export function isUserInstalledMcpServer(
  server: Pick<RuntimeMcpServer, "name" | "sourceConnectorId">,
  connectorOwnedNames: ReadonlySet<string>,
): boolean {
  return (
    !server.sourceConnectorId?.trim() &&
    !connectorOwnedNames.has(server.name) &&
    !isPluginMcpServerName(server.name)
  );
}

export function findConnectorOwnedMcpServer(
  connector: Pick<RuntimeConnector, "serverNames">,
  servers: RuntimeMcpServer[],
): RuntimeMcpServer | undefined {
  const serverNames = connector.serverNames ?? [];
  if (serverNames.length !== 1) return undefined;
  return servers.find((server) => server.name === serverNames[0]);
}

export function withMcpServerToggleState(
  servers: RuntimeMcpServer[],
  name: string,
  enabled: boolean,
): RuntimeMcpServer[] {
  const nextState: RuntimeMcpServer["state"] = enabled ? "pending" : "disabled";
  return servers.map((server) =>
    server.name === name
      ? {
          ...server,
          state: nextState,
          errorReason: "",
        }
      : server,
  );
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

function connectorBrandLogo(providerId: string): string | undefined {
  if (providerId === "dingtalk-workspace") return dingtalkLogo;
  if (providerId === "lark-cli") return feishuLogo;
  return undefined;
}

function connectorBrandTone(providerId: string): "dingtalk" | "lark" | "neutral" {
  if (providerId === "dingtalk-workspace") return "dingtalk";
  if (providerId === "lark-cli") return "lark";
  return "neutral";
}

function stateLabel(state: string): string {
  switch (state) {
    case "connected":
      return "已连接";
    case "pending":
      return "连接中";
    case "needs-auth":
      return "需授权";
    case "approval-required":
      return "等待管理员审批";
    case "failed":
      return "失败";
    case "loaded":
      return "已加载";
    case "invalid":
      return "无效";
    case "shadowed":
      return "被覆盖";
    case "disabled_by_policy":
      return "策略禁用";
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
    case "managed":
      return "管理策略";
    case "additional":
      return "附加目录";
    case "plugin":
      return "插件";
    case "agents":
      return "Agents";
    case "bundled":
      return "随附";
    case "legacy":
    case "legacy-command":
      return "旧命令";
    default:
      return source;
  }
}

function mcpConnectorCardItem(
  connector: RuntimeConnector,
  busyConnectorId: string | null,
  ownedServer: RuntimeMcpServer | undefined,
  busyMcpServerName: string | null,
  authorizing = false,
): AwakenConnectorCardItem {
  const managedServer = connector.authType === "oauth2" ? undefined : ownedServer;
  const renderedState = managedServer?.state ?? connector.state;
  return {
    id: connector.id,
    variant: "catalog",
    displayName: connector.displayName,
    description: connector.state === "approval-required" && connector.lastError?.trim()
      ? connector.lastError.trim()
      : connector.provider || connectorCategoryLabel(connector.category),
    state: renderedState,
    stateLabel: stateLabel(renderedState),
    tags: [
      connectorCategoryLabel(connector.category),
      sourceLabel(connector.sourceOrigin),
      ...(connector.serverNames ?? []),
    ],
    errors: connector.state !== "approval-required" && connector.lastError?.trim() ? [connector.lastError.trim()] : undefined,
    connected: connector.state === "connected",
    authorizing,
    showAdminApprovalAction: authorizing,
    enabled: managedServer ? managedServer.state !== "disabled" : undefined,
    mcpServerName: managedServer?.name,
    busy:
      busyConnectorId === connector.id ||
      (managedServer !== undefined && busyMcpServerName === managedServer.name),
  };
}

export function mcpServerCardItem(
  server: RuntimeMcpServer,
  busyMcpServerName: string | null,
): AwakenConnectorCardItem {
  return {
    id: server.name,
    variant: "server",
    displayName: server.displayName?.trim() || server.name,
    description: server.errorReason || `${server.transport} · ${server.scope}`,
    state: server.state,
    stateLabel: stateLabel(server.state),
    enabled: server.state !== "disabled",
    busy: busyMcpServerName === server.name,
  };
}

function cliConnectorCardItem(
  connector: RuntimeConnector,
  cliConnectors: CliConnectorView[],
  busyConnectorId: string | null,
): AwakenConnectorCardItem | null {
  const view = cliConnectors.find((entry) => entry.providerId === connector.id);
  const readiness = view?.readiness ?? connector.cliReadiness;
  if (!readiness) return null;
  const ready = readiness.overall === "ready" && readiness.authentication === "authorized";
  const product = cliConnectorProductCopy(connector.id);
  const errors = [
    readiness.safeError && !ready ? readiness.safeError.message : "",
    connector.legacyMcpReviewRequired
      ? "检测到同名旧 MCP 配置，因内容已被修改而保留。请在 MCP servers 中复核后手动处理。"
      : "",
  ].filter(Boolean);
  return {
    id: connector.id,
    variant: "cli",
    displayName: product?.displayName || connector.displayName,
    description: product?.description || connector.description?.trim() || "官方 CLI 连接器",
    brandLogoSrc: connectorBrandLogo(connector.id),
    brandTone: connectorBrandTone(connector.id),
    connected: ready,
    busy: busyConnectorId === connector.id,
    errors,
  };
}

export function pluginOwnerFromSkillId(id: string): string | undefined {
  if (!id.startsWith("plugin__")) {
    return undefined;
  }
  const rest = id.slice("plugin__".length);
  const separator = rest.indexOf("__");
  if (separator <= 0) {
    return undefined;
  }
  return rest.slice(0, separator);
}

export function skillSourceLabel(source: string, skillId?: string): string {
  switch (source) {
    case "project":
      return "项目";
    case "user":
      return "全局";
    case "bundled":
      return "Awaken 内置";
    case "plugin": {
      const owner = pluginOwnerFromSkillId(skillId ?? "");
      return owner ? `插件 · ${owner}` : "插件";
    }
    default:
      return sourceLabel(source);
  }
}

export function isSkillPackageDeletable(skill: Pick<RuntimeSkill, "sourceOrigin">): boolean {
  return skill.sourceOrigin === "project" || skill.sourceOrigin === "user";
}
export function canPreviewSkill(
  skill: Pick<RuntimeSkill, "catalogEntryId" | "skillFilePath" | "sourceOrigin">,
): boolean {
  return (
    skill.sourceOrigin !== "mcp" &&
    Boolean(skill.catalogEntryId?.trim()) &&
    Boolean(skill.skillFilePath?.trim())
  );
}

export function isSkillVisibleInView(
  skill: Pick<RuntimeSkill, "sourceOrigin">,
  view: SkillView,
): boolean {
  if (skill.sourceOrigin === "plugin") {
    return false;
  }
  switch (view) {
    case "awaken":
      return skill.sourceOrigin === "bundled";
    case "project":
      return skill.sourceOrigin === "project";
    case "agents":
      return skill.sourceOrigin === "agents";
    case "user":
      return (
        skill.sourceOrigin !== "project" &&
        skill.sourceOrigin !== "bundled" &&
        skill.sourceOrigin !== "agents"
      );
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
    matchesQuery(skill.id, query) ||
    matchesQuery(skill.skillFilePath, query) ||
    (skill.diagnostics ?? []).some((diagnostic) =>
      matchesQuery(skillDiagnosticText(diagnostic), query),
    )
  );
}

export function skillDiagnosticText(
  diagnostic: NonNullable<RuntimeSkill["diagnostics"]>[number],
): string {
  if (typeof diagnostic === "string") {
    return diagnostic;
  }
  return [diagnostic.code, diagnostic.message].filter(Boolean).join(": ");
}

function projectLabelFromRoot(root: string): string {
  const cleaned = root.trim().replace(/[\\/]+$/, "");
  if (!cleaned) return "未命名项目";
  return cleaned.split(/[\\/]/).pop() || cleaned;
}

function agentsSkillSourceRoot(rootDir: string): string | undefined {
  const cleaned = rootDir.trim().replace(/[\\/]+$/, "");
  const match = cleaned.match(/^(.+)[\\/]\.agents[\\/]skills[\\/][^\\/]+$/i);
  return match?.[1];
}

function isAgentsSkillGlobal(sourceRoot: string, projectRoots: string[]): boolean {
  const normalized = sourceRoot.trim().toLowerCase();
  return !projectRoots.some((root) => root.trim().toLowerCase() === normalized);
}

type SkillListLoadResult = {
  root?: string;
  skills: RuntimeSkill[];
  errorMessage?: string;
};

type SkillProjectLoadResult = {
  project: RuntimeProject;
  skills: RuntimeSkill[];
  errorMessage?: string;
};

type SkillCatalogSnapshot = {
  skills: RuntimeSkill[];
  projectGroups: SkillProjectGroup[];
  signature: string;
};

type McpSettingsLoadResult = {
  config?: RuntimeMcpServerConfigResponse;
  configErrorMessage?: string;
  tools: RuntimeToolInfo[];
  logs: RuntimeLogEntry[];
};

export async function loadMcpSettingsResources(
  serverName: string,
  runtimeClient: Pick<
    RuntimeClient,
    "getMcpServerConfig" | "listMcpTools" | "getRuntimeLogs"
  >,
): Promise<McpSettingsLoadResult> {
  const [configResult, toolsResult, logsResult] = await Promise.allSettled([
    runtimeClient.getMcpServerConfig(serverName),
    runtimeClient.listMcpTools(),
    runtimeClient.getRuntimeLogs(),
  ]);
  return {
    config:
      configResult.status === "fulfilled" ? configResult.value : undefined,
    configErrorMessage:
      configResult.status === "rejected"
        ? configResult.reason instanceof Error
          ? configResult.reason.message
          : "加载 MCP 配置失败"
        : undefined,
    tools: toolsResult.status === "fulfilled" ? toolsResult.value : [],
    logs: logsResult.status === "fulfilled" ? logsResult.value : [],
  };
}

export async function loadSkillListsForRoots(
  roots: Array<string | undefined>,
  listSkills: (projectRoot?: string) => Promise<RuntimeSkill[]>,
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

function orderedProjectRoots(project: RuntimeProject): string[] {
  const primary = project.folders.find((folder) => folder.isPrimary)?.path;
  return dedupePaths([
    primary,
    project.workspaceRoot,
    ...project.folders
      .filter((folder) => folder.path !== primary)
      .map((folder) => folder.path),
  ]);
}

export async function loadSkillListsForProjects(
  projects: RuntimeProject[],
  listSkills: RuntimeClient["listSkills"],
): Promise<SkillProjectLoadResult[]> {
  return Promise.all(
    projects.map(async (project) => {
      const roots = orderedProjectRoots(project);
      if (roots.length === 0) {
        return { project, skills: [] };
      }
      try {
        return { project, skills: await listSkills(roots) };
      } catch (error) {
        return {
          project,
          skills: [],
          errorMessage:
            error instanceof Error ? error.message : "项目 Skills 加载失败",
        };
      }
    }),
  );
}

function buildSkillCatalogSnapshot(
  results: SkillProjectLoadResult[],
  fallbackSkills: RuntimeSkill[] = [],
): SkillCatalogSnapshot {
  const skills = dedupeSkills([
    ...results.flatMap((result) => result.skills),
    ...fallbackSkills,
  ]).filter((skill) =>
    SKILL_VIEW_TABS.some((view) => isSkillVisibleInView(skill, view.id)),
  );
  const projectGroups = results.map(({ project, skills: projectSkills, errorMessage }) => {
    const roots = orderedProjectRoots(project);
    const primaryRoot = roots[0] ?? "";
    const folderByRoot = new Map(
      project.folders.map((folder) => [folder.path, folder]),
    );
    return {
      id: project.projectId,
      root: primaryRoot,
      label: project.displayName.trim() || projectLabelFromRoot(primaryRoot),
      folders: roots.map((root) => ({
        root,
        label: projectLabelFromRoot(root),
        isPrimary: Boolean(folderByRoot.get(root)?.isPrimary),
      })),
      skills: projectSkills.filter(
      (skill) => skill.sourceOrigin === "project",
      ),
      errorMessage,
    };
  });
  return {
    skills,
    projectGroups,
    signature: JSON.stringify({ skills, projectGroups }),
  };
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

export function dedupeSkills(skills: RuntimeSkill[]): RuntimeSkill[] {
  const seen = new Set<string>();
  const result: RuntimeSkill[] = [];
  for (const skill of skills) {
    const key = `${skill.id}\x00${skill.sourceOrigin}\x00${skill.rootDir ?? ""}`;
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    result.push(skill);
  }
  return result;
}

function skillCatalogIdentity(skill: RuntimeSkill, projectRoot?: string): string {
  return skill.catalogEntryId?.trim()
    || `${skill.id}:${skill.sourceOrigin}:${projectRoot ?? skill.rootDir ?? ""}`;
}

export const SKILL_CATALOG_SYNC_INTERVAL_MS = 1_000;

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

function fileNameFromLocalPath(value: string): string {
  const normalized = value.trim().replace(/\\/g, "/");
  return normalized.split("/").pop() || normalized;
}

export function ExtensionsView({
  section,
  desktopReady,
  runtimeClient,
  initialPlugins = [],
  onNotice,
  activeWorkspaceRoot,
  composerCatalogRevision = 0,
  navigationEpoch = 0,
  onNavigateSection,
  onUsePlugin = NOOP_USE_PLUGIN,
  onUsePluginComponent = NOOP_USE_PLUGIN_COMPONENT,
}: ExtensionsViewProps) {
  const [connectors, setConnectors] = useState<RuntimeConnector[]>([]);
  const [cliConnectors, setCliConnectors] = useState<CliConnectorView[]>([]);
  const [mcpServers, setMcpServers] = useState<RuntimeMcpServer[]>([]);
  const [skills, setSkills] = useState<RuntimeSkill[]>([]);
  const [skillProjectGroups, setSkillProjectGroups] = useState<SkillProjectGroup[]>([]);
  const [query, setQuery] = useState("");
  const [activeConnectorKind, setActiveConnectorKind] =
    useState<ConnectorKind>("mcp");
  const [activeSkillView, setActiveSkillView] = useState<SkillView>(
    DEFAULT_SKILL_VIEW,
  );
  const [openProjectRoot, setOpenProjectRoot] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [busyConnectorId, setBusyConnectorId] = useState<string | null>(null);
  const [connectorAuthSessions, setConnectorAuthSessions] = useState<Record<string, string>>({});
  const [cliDetailConnectorId, setCliDetailConnectorId] = useState<CliProviderId | null>(null);
  const [cliDetailLoading, setCliDetailLoading] = useState(false);
  const [busyMcpServerName, setBusyMcpServerName] = useState<string | null>(null);
  const [mcpDialogOpen, setMcpDialogOpen] = useState(false);
  const [mcpConfigMode, setMcpConfigMode] = useState<AwakenMcpConfigMode>("stdio");
  const [mcpConfigText, setMcpConfigText] = useState(MCP_STDIO_EXAMPLE);
  const [configuringMcp, setConfiguringMcp] = useState(false);
  const [skillDialogOpen, setSkillDialogOpen] = useState(false);
  const [skillFile, setSkillFile] = useState<File | null>(null);
  const [skillSourcePath, setSkillSourcePath] = useState("");
  const [classifyingSkillPackage, setClassifyingSkillPackage] = useState(false);
  const [skillDragActive, setSkillDragActive] = useState(false);
  const [installingSkill, setInstallingSkill] = useState(false);
  const [skillProjectRoot, setSkillProjectRoot] = useState("");
  const [skillInstallScope, setSkillInstallScope] =
    useState<SkillInstallScope>("project");
  const [skillInstallProjectRoot, setSkillInstallProjectRoot] = useState("");
  const [pickingSkillProjectRoot, setPickingSkillProjectRoot] = useState(false);
  const [skillPreviewTarget, setSkillPreviewTarget] =
    useState<SkillPreviewTarget | null>(null);
  const projectSkillPreviewScrollRef = useRef<HTMLDivElement>(null);
  const skillSnapshotSignatureRef = useRef("");
  const skillSyncInFlightRef = useRef(false);
  const refreshGenerationRef = useRef(0);
  const visibleRefreshGenerationRef = useRef(0);
  const appliedPluginCatalogRevisionRef = useRef(composerCatalogRevision);
  const pluginValidationGenerationRef = useRef(0);
  const skillPackageRoutingGenerationRef = useRef(0);
  const busyMcpServerNameRef = useRef<string | null>(null);
  const [skillToggleStates, setSkillToggleStates] = useState<Record<string, boolean>>({});
  const [busySkillId, setBusySkillId] = useState<string | null>(null);
  const [pendingSkillDelete, setPendingSkillDelete] =
    useState<PendingSkillDelete | null>(null);
  const [deletingSkill, setDeletingSkill] = useState(false);
  const [skillDeleteError, setSkillDeleteError] = useState<string | null>(null);
  const [mcpSettingsOpen, setMcpSettingsOpen] = useState(false);
  const [selectedMcpServer, setSelectedMcpServer] = useState<RuntimeMcpServer | null>(null);
  const [mcpSettingsConfigText, setMcpSettingsConfigText] = useState("");
  const [mcpSettingsScope, setMcpSettingsScope] = useState("");
  const [mcpSettingsError, setMcpSettingsError] = useState<string | null>(null);
  const [mcpTools, setMcpTools] = useState<McpToolInfo[]>([]);
  const [mcpLogs, setMcpLogs] = useState<RuntimeLogEntry[]>([]);
  const [savingMcpConfig, setSavingMcpConfig] = useState(false);
  const [loadingMcpSettings, setLoadingMcpSettings] = useState(false);
  const [plugins, setPlugins] = useState<RuntimePlugin[]>(() => initialPlugins);
  const [activePluginView, setActivePluginView] = useState<AwakenPluginsView>("builtin");
  const [pluginDialogOpen, setPluginDialogOpen] = useState(false);
  const [pluginLocalPath, setPluginLocalPath] = useState("");
  const [pluginValidation, setPluginValidation] =
    useState<RuntimePluginValidation | null>(null);
  const [validatingPlugin, setValidatingPlugin] = useState(false);
  const [installingPlugin, setInstallingPlugin] = useState(false);
  const [selectedPlugin, setSelectedPlugin] = useState<RuntimePlugin | null>(null);
  const [pendingPluginUninstall, setPendingPluginUninstall] = useState<RuntimePlugin | null>(null);
  const [busyPluginName, setBusyPluginName] = useState<string | null>(null);

  useEffect(() => {
    setSelectedPlugin(null);
    setPluginDialogOpen(false);
    setPendingPluginUninstall(null);
  }, [navigationEpoch]);

  useEffect(() => {
    if (initialPlugins.length > 0) {
      setPlugins(initialPlugins);
    }
  }, [initialPlugins]);

  const normalizedQuery = query.trim().toLowerCase();
  function applySkillCatalogSnapshot(snapshot: SkillCatalogSnapshot): void {
    if (skillSnapshotSignatureRef.current === snapshot.signature) {
      return;
    }
    skillSnapshotSignatureRef.current = snapshot.signature;
    setSkills(snapshot.skills);
    setSkillProjectGroups(snapshot.projectGroups);
  }

  async function refresh(showLoading = true, projectRootOverride?: string): Promise<void> {
    if (!desktopReady) {
      setErrorMessage("Awaken runtime 未启动");
      return;
    }
    const generation = ++refreshGenerationRef.current;
    const visibleGeneration = showLoading
      ? ++visibleRefreshGenerationRef.current
      : null;
    if (showLoading) {
      setLoading(true);
    }
    setErrorMessage(null);
    try {
      const [runtimeInfo, runtimeProjects] = await Promise.all([
        runtimeClient.getRuntimeInfo(),
        runtimeClient.listProjects(false),
      ]);
      if (generation !== refreshGenerationRef.current) {
        return;
      }
      const projectRoots = dedupePaths(
        runtimeProjects.flatMap((project) => orderedProjectRoots(project)),
      );
      const fallbackRoots = dedupePaths([
        projectRootOverride,
        skillProjectRoot,
        activeWorkspaceRoot,
        runtimeInfo?.defaultWorkspace,
      ]).filter((root) => !projectRoots.includes(root));
      const needsFallbackCatalog = projectRoots.length === 0 || fallbackRoots.length > 0;
      const [
        nextConnectors,
        nextCliConnectors,
        nextMcpServers,
        nextProjectSkillResults,
        fallbackSkills,
        nextPlugins,
      ] = await Promise.all([
        runtimeClient.listConnectors(),
        runtimeClient.listCliConnectors(),
        runtimeClient.listMcpServers(),
        loadSkillListsForProjects(runtimeProjects, runtimeClient.listSkills),
        needsFallbackCatalog
          ? runtimeClient.listSkills(fallbackRoots.length > 0 ? fallbackRoots : undefined)
          : Promise.resolve([]),
        typeof runtimeClient.listPlugins === "function"
          ? runtimeClient.listPlugins()
          : Promise.resolve(plugins),
      ]);
      if (generation !== refreshGenerationRef.current) {
        return;
      }
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
      setCliConnectors(nextCliConnectors);
      setPlugins(nextPlugins);
      setSelectedPlugin((current) =>
        current
          ? nextPlugins.find((plugin) => plugin.name === current.name) ?? null
          : null,
      );
      const busyServerName = busyMcpServerNameRef.current;
      setMcpServers((current) => {
        if (!busyServerName) {
          return nextMcpServers;
        }
        const busyCurrent = current.find((server) => server.name === busyServerName);
        const busyNext = nextMcpServers.find((server) => server.name === busyServerName);
        if (busyCurrent && !busyNext) {
          // Keep the card in place while a toggle/restart briefly omits it.
          return [
            ...nextMcpServers,
            busyCurrent,
          ];
        }
        return nextMcpServers;
      });
      applySkillCatalogSnapshot(
        buildSkillCatalogSnapshot(nextProjectSkillResults, fallbackSkills),
      );
    } catch (error) {
      if (generation !== refreshGenerationRef.current) {
        return;
      }
      setErrorMessage(error instanceof Error ? error.message : "扩展列表加载失败");
    } finally {
      // Silent polling may supersede the data snapshot, but it must not steal
      // ownership of a visible refresh indicator and leave the button spinning.
      if (
        visibleGeneration !== null &&
        visibleGeneration === visibleRefreshGenerationRef.current
      ) {
        setLoading(false);
      }
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [desktopReady]);

  useEffect(() => {
    if (!desktopReady) return;
    return runtimeClient.subscribeCliConnectorState(({ connector }) => {
      setCliConnectors((current) => [
        ...current.filter((entry) => entry.providerId !== connector.providerId),
        connector,
      ].sort((a, b) => a.providerId.localeCompare(b.providerId)));
      setConnectors((current) => current.map((entry) =>
        entry.id === connector.providerId ? { ...entry, cliReadiness: connector.readiness } : entry,
      ));
    });
  }, [desktopReady, runtimeClient]);

  useEffect(() => {
    if (!desktopReady || !activeWorkspaceRoot?.trim()) {
      return;
    }
    void refresh(false, activeWorkspaceRoot);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeWorkspaceRoot]);

  useEffect(() => {
    if (
      !desktopReady ||
      section !== "plugins" ||
      composerCatalogRevision <= appliedPluginCatalogRevisionRef.current
    ) {
      return;
    }
    appliedPluginCatalogRevisionRef.current = composerCatalogRevision;
    void refresh(false);
    // Store mutations publish this revision only after Plugin state and live
    // roots are committed, so the visible catalog never has to preserve an
    // earlier digest diagnostic after an authorized commit.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [composerCatalogRevision, desktopReady, section]);

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
    if (!desktopReady || section !== "skills") {
      return;
    }
    let disposed = false;
    const syncSkillsFromDisk = async (): Promise<void> => {
      if (skillSyncInFlightRef.current) {
        return;
      }
      skillSyncInFlightRef.current = true;
      try {
        const runtimeProjects = await runtimeClient.listProjects(false);
        const results = await loadSkillListsForProjects(
          runtimeProjects,
          runtimeClient.listSkills,
        );
        if (disposed || results.some((result) => result.errorMessage)) {
          return;
        }
        const hasProjectFolders = runtimeProjects.some(
          (project) => orderedProjectRoots(project).length > 0,
        );
        const fallbackResults = hasProjectFolders
          ? []
          : await loadSkillListsForRoots([undefined], runtimeClient.listSkills);
        if (disposed || fallbackResults.some((result) => result.errorMessage)) {
          return;
        }
        applySkillCatalogSnapshot(
          buildSkillCatalogSnapshot(results, fallbackResults[0]?.skills ?? []),
        );
      } finally {
        skillSyncInFlightRef.current = false;
      }
    };
    const timer = setInterval(() => {
      void syncSkillsFromDisk();
    }, SKILL_CATALOG_SYNC_INTERVAL_MS);
    return () => {
      disposed = true;
      clearInterval(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [desktopReady, runtimeClient, section]);

  useEffect(() => {
    if (!skillPreviewTarget) {
      return;
    }
    const previewExists = skillPreviewTarget.projectRoot
      ? skillProjectGroups.some(
          (group) =>
            group.skills.some(
              (skill) =>
                skill.catalogEntryId === skillPreviewTarget.catalogEntryId &&
                (skill.projectPath || group.root) === skillPreviewTarget.projectRoot,
            ),
        )
      : skills.some(
          (skill) =>
            skill.catalogEntryId === skillPreviewTarget.catalogEntryId,
        );
    if (!previewExists) {
      setSkillPreviewTarget(null);
    }
  }, [skillPreviewTarget, skillProjectGroups, skills]);

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
          isConnectorVisibleInCatalog(connector) &&
          (matchesQuery(connector.displayName, normalizedQuery) ||
            matchesQuery(connector.provider, normalizedQuery) ||
            matchesQuery(connector.id, normalizedQuery)),
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

  const connectorOwnedMcpServers = useMemo(
    () => connectorOwnedMcpServerNames(connectors),
    [connectors],
  );

  const visibleMcpServers = useMemo(
    () =>
      mcpServers.filter(
        (server) =>
          isUserInstalledMcpServer(server, connectorOwnedMcpServers) &&
          (matchesQuery(server.name, normalizedQuery) ||
            matchesQuery(server.transport, normalizedQuery) ||
            matchesQuery(server.errorReason, normalizedQuery)),
      ),
    [connectorOwnedMcpServers, mcpServers, normalizedQuery],
  );

  const visibleSkills = useMemo(
    () =>
      skills.filter(
        (skill) =>
          isSkillVisibleInView(skill, activeSkillView) &&
          skillMatchesQuery(skill, normalizedQuery),
      ),
    [activeSkillView, skills, normalizedQuery],
  );

  const visibleProjectSkillGroups = useMemo<VisibleSkillProjectGroup[]>(
    () =>
      skillProjectGroups
        .map((group) => {
          const projectMatches =
            matchesQuery(group.label, normalizedQuery) ||
            group.folders.some((folder) =>
              matchesQuery(folder.root, normalizedQuery),
            );
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
            group.folders.some((folder) =>
              matchesQuery(folder.root, normalizedQuery),
            ) ||
            group.visibleSkills.length > 0,
        ),
    [skillProjectGroups, normalizedQuery],
  );

  const openProjectSkillGroup = visibleProjectSkillGroups.find(
    (group) => group.id === openProjectRoot,
  );

  const openProjectSkillDialog = useMemo<AwakenSkillProject | null>(() => {
    if (!openProjectSkillGroup) {
      return null;
    }
    return {
      id: openProjectSkillGroup.id,
      name: openProjectSkillGroup.label,
      path: openProjectSkillGroup.root,
      folders: openProjectSkillGroup.folders.map((folder) => ({
        path: folder.root,
        label: folder.label,
        isPrimary: folder.isPrimary,
      })),
      skills: openProjectSkillGroup.visibleSkills.map((skill): AwakenSkillItem => {
        const projectRoot = skill.projectPath || openProjectSkillGroup.root;
        const skillKey = `${skill.id}:${skill.sourceOrigin}:${projectRoot}`;
        const sourceFolder = openProjectSkillGroup.folders.find(
          (folder) => folder.root === projectRoot,
        );
        const diagnostics = (skill.diagnostics ?? [])
          .map(skillDiagnosticText)
          .filter(Boolean);
        return {
          id: skillCatalogIdentity(skill, projectRoot),
          name: skill.displayName || skill.name,
          description: skill.whenToUse || skill.description,
          enabled:
            skillToggleStates[skillKey] ?? skill.state !== "disabled_by_policy",
          scope: "project",
          sourceLabel: sourceFolder
            ? sourceFolder.label
            : skillSourceLabel(skill.sourceOrigin, skill.id),
          diagnostic: diagnostics.length > 0 ? diagnostics.join(" · ") : undefined,
          previewable: canPreviewSkill(skill),
          busy: busySkillId === skill.id,
          toggleDisabled: busySkillId === skill.id || deletingSkill,
          projectId: projectRoot,
          folderPath: projectRoot,
        };
      }),
    };
  }, [
    busySkillId,
    deletingSkill,
    openProjectSkillGroup,
    skillToggleStates,
  ]);

  const projectSkillPreviewTarget = skillPreviewTarget && openProjectSkillGroup?.skills.some(
    (skill) => skill.catalogEntryId === skillPreviewTarget.catalogEntryId,
  ) ? skillPreviewTarget : null;
  const projectSkillDeleteTarget = pendingSkillDelete && openProjectSkillGroup?.skills.some(
    (skill) =>
      skill.id === pendingSkillDelete.skill.id &&
      (skill.projectPath || openProjectSkillGroup.root) === pendingSkillDelete.projectRoot,
  ) ? pendingSkillDelete : null;
  const projectSkillPreviewSkill = projectSkillPreviewTarget
    ? openProjectSkillGroup?.skills.find(
        (skill) => skill.catalogEntryId === projectSkillPreviewTarget.catalogEntryId,
      ) ?? null
    : null;
  const standaloneSkillPreviewSkill = skillPreviewTarget && !skillPreviewTarget.projectRoot
    ? skills.find(
        (skill) => skill.catalogEntryId === skillPreviewTarget.catalogEntryId,
      ) ?? null
    : null;

  const visibleUserSkills = useMemo(
    () => visibleSkills.filter((skill) => skill.sourceOrigin === "user"),
    [visibleSkills],
  );

  const visibleAwakenSkills = useMemo(
    () => visibleSkills.filter((skill) => skill.sourceOrigin === "bundled"),
    [visibleSkills],
  );

  const visibleMcpSkills = useMemo(
    () => visibleSkills.filter((skill) => skill.sourceOrigin === "mcp"),
    [visibleSkills],
  );

  const visibleOtherSkills = useMemo(
    () =>
      visibleSkills.filter(
        (skill) =>
          skill.sourceOrigin !== "project" &&
          skill.sourceOrigin !== "user" &&
          skill.sourceOrigin !== "agents" &&
          skill.sourceOrigin !== "mcp" &&
          skill.sourceOrigin !== "bundled",
      ),
    [visibleSkills],
  );

  const visibleAgentsSkills = useMemo<VisibleAgentsSkill[]>(() => {
    const projectRoots = skillProjectGroups.flatMap((group) =>
      group.folders.map((folder) => folder.root),
    );
    return visibleSkills.flatMap((skill) => {
      if (skill.sourceOrigin !== "agents") {
        return [];
      }
      const sourceRoot = agentsSkillSourceRoot(skill.rootDir ?? "");
      if (!sourceRoot) {
        return [];
      }
      return [{
        skill,
        projectRoot: isAgentsSkillGlobal(sourceRoot, projectRoots)
          ? undefined
          : sourceRoot,
      }];
    });
  }, [visibleSkills, skillProjectGroups]);

  async function handleConnectorAction(connector: RuntimeConnector): Promise<void> {
    setBusyConnectorId(connector.id);
    setErrorMessage(null);
    try {
      const activeSessionId = connectorAuthSessions[connector.id];
      if (activeSessionId) {
        if (!runtimeClient.cancelConnectorAuthSession) {
          throw new Error("当前 Desktop 不支持取消连接器授权");
        }
        await runtimeClient.cancelConnectorAuthSession(activeSessionId);
        setConnectorAuthSessions((current) => {
          const next = { ...current };
          delete next[connector.id];
          return next;
        });
        onNotice(`${connector.displayName} 授权已取消`);
        await refresh();
        return;
      }
      if (connector.state === "connected") {
        const result = await runtimeClient.disconnectConnector(connector.id);
        onNotice(result.message ?? `${connector.displayName} 已断开`);
      } else {
        const result = await runtimeClient.connectConnector(connector.id);
        onNotice(result.message ?? `${connector.displayName} 已连接`);
        if (result.sessionId) {
          setConnectorAuthSessions((current) => ({ ...current, [connector.id]: result.sessionId! }));
          void pollConnectorAuthSession(connector, result.sessionId);
        }
      }
      await refresh();
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "连接器操作失败");
    } finally {
      setBusyConnectorId(null);
    }
  }

  async function pollConnectorAuthSession(connector: RuntimeConnector, sessionId: string): Promise<void> {
    if (!runtimeClient.getConnectorAuthSession) {
      setErrorMessage("当前 Desktop 不支持连接器授权会话");
      return;
    }
    for (;;) {
      await new Promise((resolve) => window.setTimeout(resolve, 750));
      try {
        const session = await runtimeClient.getConnectorAuthSession(sessionId);
        if (session.status === "authorizing" || session.status === "verifying") continue;
        setConnectorAuthSessions((current) => {
          if (current[connector.id] !== sessionId) return current;
          const next = { ...current };
          delete next[connector.id];
          return next;
        });
        if (session.status === "ready") {
          onNotice(`${connector.displayName} 已连接`);
        } else if (session.status === "pending_admin_approval") {
          onNotice(`${connector.displayName} 正在等待组织管理员批准；批准后请重新连接`);
        } else if (session.status !== "cancelled") {
          setErrorMessage(session.errorMessage?.trim() || `${connector.displayName} 授权未完成`);
        }
        await refresh();
        return;
      } catch (error) {
        setConnectorAuthSessions((current) => {
          if (current[connector.id] !== sessionId) return current;
          const next = { ...current };
          delete next[connector.id];
          return next;
        });
        setErrorMessage(error instanceof Error ? error.message : "连接器授权状态读取失败");
        return;
      }
    }
  }

  async function handleConnectorAdminApprovalRequired(connector: RuntimeConnector): Promise<void> {
    const sessionId = connectorAuthSessions[connector.id];
    if (!sessionId) {
      setErrorMessage("当前没有可标记的连接器授权会话");
      return;
    }
    if (!runtimeClient.markConnectorAdminApprovalPending) {
      setErrorMessage("当前 Desktop 不支持管理员审批状态");
      return;
    }
    setBusyConnectorId(connector.id);
    setErrorMessage(null);
    try {
      await runtimeClient.markConnectorAdminApprovalPending(sessionId);
      setConnectorAuthSessions((current) => {
        const next = { ...current };
        delete next[connector.id];
        return next;
      });
      onNotice(`${connector.displayName} 已记录为等待组织管理员审批`);
      await refresh();
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "管理员审批状态保存失败");
    } finally {
      setBusyConnectorId(null);
    }
  }

  async function handleCliAction(providerId: CliProviderId, action: () => Promise<CliConnectorView>): Promise<void> {
    setBusyConnectorId(providerId);
    setErrorMessage(null);
    try {
      const next = await action();
      setCliConnectors((current) => [
        ...current.filter((entry) => entry.providerId !== providerId),
        next,
      ].sort((a, b) => a.providerId.localeCompare(b.providerId)));
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "CLI 操作失败");
    } finally {
      setBusyConnectorId(null);
    }
  }

  async function handleToggleMcpServer(server: RuntimeMcpServer): Promise<void> {
    const enabling = server.state === "disabled";
    busyMcpServerNameRef.current = server.name;
    setBusyMcpServerName(server.name);
    setErrorMessage(null);
    setMcpServers((current) => withMcpServerToggleState(current, server.name, enabling));
    try {
      if (enabling) {
        await runtimeClient.enableMcpServer(server.name);
        onNotice(`${server.name} 已启用`);
      } else {
        await runtimeClient.disableMcpServer(server.name);
        onNotice(`${server.name} 已停用`);
      }
      await refresh(false);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "MCP server 操作失败");
      await refresh(false);
    } finally {
      busyMcpServerNameRef.current = null;
      setBusyMcpServerName(null);
    }
  }

  async function handleConfigureMcp(): Promise<void> {
    if (mcpConfigText.trim() === "") {
      setErrorMessage("请粘贴 MCP 配置 JSON 或 Streamable HTTP URL");
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

  function handleMcpConfigModeChange(nextMode: AwakenMcpConfigMode): void {
    setMcpConfigText((currentValue) =>
      nextMcpConfigTextForMode(currentValue, nextMode));
    setMcpConfigMode(nextMode);
    setErrorMessage(null);
  }

  function handleMcpConfigTextChange(nextValue: string): void {
    setMcpConfigText(nextValue);
    const inferredMode = inferMcpConfigMode(nextValue);
    if (inferredMode) {
      setMcpConfigMode(inferredMode);
    }
    setErrorMessage(null);
  }

  async function loadMcpSettings(server: RuntimeMcpServer): Promise<void> {
    setSelectedMcpServer(server);
    setMcpSettingsOpen(true);
    setLoadingMcpSettings(true);
    setMcpSettingsConfigText("");
    setMcpSettingsScope("");
    setMcpSettingsError(null);
    setMcpTools([]);
    setMcpLogs([]);
    try {
      const resources = await loadMcpSettingsResources(server.name, runtimeClient);
      if (resources.config) {
        setMcpSettingsConfigText(resources.config.configText);
        setMcpSettingsScope(resources.config.scope);
      }
      setMcpSettingsError(resources.configErrorMessage ?? null);
      setMcpTools(extractServerTools(resources.tools, server.name));
      setMcpLogs(filterMcpLogs(resources.logs, server.name));
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
      setMcpSettingsError("MCP 配置不能为空");
      return;
    }
    setSavingMcpConfig(true);
    setMcpSettingsError(null);
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
      setMcpSettingsError(error instanceof Error ? error.message : "保存 MCP 配置失败");
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
    setMcpSettingsError(null);
    try {
      await runtimeClient.deleteMcpServer(selectedMcpServer.name);
      onNotice(`已删除 ${selectedMcpServer.name}`);
      setMcpSettingsOpen(false);
      setSelectedMcpServer(null);
      await refresh();
    } catch (error) {
      setMcpSettingsError(error instanceof Error ? error.message : "删除 MCP server 失败");
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
    if (!skillFile && !skillSourcePath) {
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
      const contentBase64 = skillFile ? await fileToBase64(skillFile) : undefined;
      const result = await runtimeClient.installSkill({
        fileName: skillFile?.name ?? fileNameFromLocalPath(skillSourcePath),
        contentBase64,
        sourcePath: skillSourcePath || undefined,
        scope: skillInstallScope,
        projectRoot: targetProjectRoot || undefined,
      });
      setSkillDialogOpen(false);
      setSkillFile(null);
      setSkillSourcePath("");
      if (skillInstallScope === "project") {
        setSkillProjectRoot(targetProjectRoot);
      }
      onNotice(`已安装 ${result.skill.displayName || result.skill.name}`);
      await refresh(true, targetProjectRoot || undefined);
    } catch (error) {
      if (
        pluginErrorCode(error) === "PLUGIN_PACKAGE_SELECTED" &&
        skillFile &&
        await openPluginInstallerForSkillFile(skillFile, true)
      ) {
        return;
      }
      setErrorMessage(error instanceof Error ? error.message : "Skill 安装失败");
    } finally {
      setInstallingSkill(false);
    }
  }

  async function handleDeleteSkill(): Promise<void> {
    if (!pendingSkillDelete) {
      return;
    }
    const { skill, projectRoot } = pendingSkillDelete;
    setDeletingSkill(true);
    setSkillDeleteError(null);
    setErrorMessage(null);
    try {
      await runtimeClient.deleteSkill({
        id: skill.id,
        sourceOrigin: skill.sourceOrigin,
        projectRoot: skill.sourceOrigin === "project" ? projectRoot : undefined,
      });
      onNotice(`已删除 ${skill.displayName || skill.name}，Skill 包已移到废纸篓`);
      await refresh(true, projectRoot);
      setPendingSkillDelete(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Skill 删除失败";
      setSkillDeleteError(message);
      setErrorMessage(message);
    } finally {
      setDeletingSkill(false);
    }
  }

  function cancelSkillDelete(): void {
    setPendingSkillDelete(null);
    setSkillDeleteError(null);
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

  function openRoutedSkillInstaller(sourcePath: string): void {
    invalidatePluginValidation();
    setPluginDialogOpen(false);
    setPluginLocalPath("");
    setSkillFile(null);
    setSkillSourcePath(sourcePath);
    setSkillInstallScope("project");
    setSkillInstallProjectRoot(skillProjectRoot);
    setSkillDialogOpen(true);
    setErrorMessage(null);
    onNavigateSection?.("skills");
    onNotice("已识别为 Skill 包，请确认安装范围");
  }

  async function openPluginInstallerForSkillFile(
    file: File,
    forcePluginRoute = false,
  ): Promise<boolean> {
    if (!file.name.toLowerCase().endsWith(".zip")) return false;
    const generation = ++skillPackageRoutingGenerationRef.current;
    setClassifyingSkillPackage(true);
    try {
      const resolved = await runtimeClient.resolveDroppedPluginSource(file);
      if (generation !== skillPackageRoutingGenerationRef.current || resolved.canceled) {
        return false;
      }
      if (!forcePluginRoute) {
        try {
          const validation = await runtimeClient.validatePlugin(pluginInstallSource(resolved.path));
          if (generation !== skillPackageRoutingGenerationRef.current) return false;
          setSkillDialogOpen(false);
          setSkillFile(null);
          setSkillSourcePath("");
          setPluginLocalPath(resolved.path);
          setPluginValidation(validation);
          setErrorMessage(validation.valid ? null : "插件 ZIP 格式不符合要求");
          setPluginDialogOpen(true);
          onNavigateSection?.("plugins");
          onNotice("已识别为插件包，已切换到插件安装");
          return true;
        } catch (error) {
          if (pluginErrorCode(error) === "SKILL_PACKAGE_SELECTED") return false;
          return false;
        }
      }
      setSkillDialogOpen(false);
      setSkillFile(null);
      setSkillSourcePath("");
      setPluginDialogOpen(true);
      onNavigateSection?.("plugins");
      await selectAndValidatePluginSource(resolved.path);
      onNotice("已识别为插件包，已切换到插件安装");
      return true;
    } catch (error) {
      if (forcePluginRoute) {
        setErrorMessage(pluginErrorMessage(error));
      }
      return false;
    } finally {
      if (generation === skillPackageRoutingGenerationRef.current) {
        setClassifyingSkillPackage(false);
      }
    }
  }

  async function handleSkillFileSelected(file: File | null): Promise<void> {
    skillPackageRoutingGenerationRef.current += 1;
    setClassifyingSkillPackage(false);
    setSkillFile(file);
    setSkillSourcePath("");
    setErrorMessage(null);
    if (file) {
      await openPluginInstallerForSkillFile(file);
    }
  }

  async function selectAndValidatePluginSource(path: string): Promise<void> {
    invalidatePluginValidation();
    setPluginLocalPath(path);
    await handleValidatePlugin(path);
  }

  async function handlePickPluginSource(): Promise<void> {
    setErrorMessage(null);
    try {
      const result = await runtimeClient.pickPluginSource(
        pluginLocalPath.trim() || undefined,
      );
      if (result.canceled) {
        if (result.error) setErrorMessage(pluginErrorMessage(result.error));
        return;
      }
      await selectAndValidatePluginSource(result.path);
    } catch (error) {
      setErrorMessage(pluginErrorMessage(error));
    }
  }

  async function handleDropPluginSource(file: File): Promise<void> {
    invalidatePluginValidation();
    try {
      const result = await runtimeClient.resolveDroppedPluginSource(file);
      if (result.canceled) {
        setPluginLocalPath("");
        setErrorMessage(result.error ? pluginErrorMessage(result.error) : "插件 ZIP 格式不符合要求");
        return;
      }
      await selectAndValidatePluginSource(result.path);
    } catch (error) {
      setPluginLocalPath("");
      setErrorMessage(pluginErrorMessage(error));
    }
  }

  function invalidatePluginValidation(): void {
    pluginValidationGenerationRef.current += 1;
    setPluginValidation(null);
    setValidatingPlugin(false);
    setErrorMessage(null);
  }

  async function openCliConnectorDetail(providerId: CliProviderId): Promise<void> {
    setCliDetailConnectorId(providerId);
    setCliDetailLoading(true);
    try {
      const next = await runtimeClient.refreshCliConnector(providerId);
      setCliConnectors((current) =>
        [...current.filter((entry) => entry.providerId !== providerId), next].sort((a, b) =>
          a.providerId.localeCompare(b.providerId),
        ),
      );
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "无法刷新 CLI 连接状态");
    } finally {
      setCliDetailLoading(false);
    }
  }

  function startCliInstall(providerId: CliProviderId): void {
    setErrorMessage(null);
    void runtimeClient.installCliConnector(providerId).then((next) => {
      setCliConnectors((current) => [
        ...current.filter((entry) => entry.providerId !== providerId),
        next,
      ].sort((a, b) => a.providerId.localeCompare(b.providerId)));
    }).catch((error) => {
      setErrorMessage(error instanceof Error ? error.message : "CLI 安装失败");
    });
  }

  function handleCliDialogAction(providerId: CliProviderId, action: AwakenCliConnectorAction): void {
    if (action === "install") {
      startCliInstall(providerId);
      return;
    }
    if (action === "cancel_install") {
      void handleCliAction(providerId, () => runtimeClient.cancelCliInstall(providerId));
      return;
    }
    if (action === "authorize") {
      void handleCliAction(providerId, () => runtimeClient.startCliAuth(providerId));
      return;
    }
    if (action === "cancel_authorization") {
      void handleCliAction(providerId, () => runtimeClient.cancelCliAuth(providerId));
      return;
    }
    if (action === "retry_authorization") {
      void handleCliAction(providerId, () => runtimeClient.retryCliAuth(providerId));
      return;
    }
    if (action === "logout") {
      void handleCliAction(providerId, () => runtimeClient.logoutCliConnector(providerId));
    }
  }

  async function handleToggleSkill(
    skill: RuntimeSkill,
    skillKey: string,
    nextEnabled: boolean,
  ): Promise<void> {
    if (busySkillId === skill.id) {
      return;
    }
    setBusySkillId(skill.id);
    setSkillToggleStates((current) => ({
      ...current,
      [skillKey]: nextEnabled,
    }));
    setErrorMessage(null);
    try {
      if (nextEnabled) {
        await runtimeClient.enableSkill(skill.id);
        onNotice(`${skill.displayName || skill.name} 已启用`);
      } else {
        await runtimeClient.disableSkill(skill.id);
        onNotice(`${skill.displayName || skill.name} 已停用`);
      }
      await refresh(false);
    } catch (error) {
      setSkillToggleStates((current) => ({
        ...current,
        [skillKey]: !nextEnabled,
      }));
      setErrorMessage(error instanceof Error ? error.message : "Skill 操作失败");
    } finally {
      setBusySkillId(null);
    }
  }

  function openSkillPreview(
    skill: RuntimeSkill,
    projectRoot?: string,
    returnFocusSkillId = skillCatalogIdentity(skill, projectRoot),
  ): void {
    if (!canPreviewSkill(skill)) {
      return;
    }
    setSkillPreviewTarget({
      catalogEntryId: skill.catalogEntryId!,
      projectRoot,
      skillId: returnFocusSkillId,
    });
  }

  function requestSkillDelete(skill: RuntimeSkill, projectRoot?: string): void {
    if (!isSkillPackageDeletable(skill)) {
      return;
    }
    setSkillDeleteError(null);
    setPendingSkillDelete({ skill, projectRoot });
  }

  function createSkillPreviewPresentation(
    skill: RuntimeSkill,
    projectRoot?: string,
  ): SkillPreviewPresentation {
    const skillKey = `${skill.id}:${skill.sourceOrigin}:${projectRoot ?? ""}`;
    const deletingThisSkill = Boolean(
      deletingSkill
      && pendingSkillDelete?.skill.id === skill.id
      && pendingSkillDelete.projectRoot === projectRoot,
    );
    return {
      name: skill.displayName || skill.name,
      sourceLabel: skillSourceLabel(skill.sourceOrigin, skill.id),
      description: skill.whenToUse || skill.description,
      enabled:
        skillToggleStates[skillKey] ?? skill.state !== "disabled_by_policy",
      toggleDisabled: busySkillId === skill.id || deletingThisSkill,
      deletable: isSkillPackageDeletable(skill),
      deleting: deletingThisSkill,
    };
  }

  function renderSkillCard(skill: RuntimeSkill, projectRoot?: string) {
    const diagnostics = (skill.diagnostics ?? [])
      .map(skillDiagnosticText)
      .filter(Boolean);
    const previewable = canPreviewSkill(skill);
    const skillName = skill.displayName || skill.name;
    const skillKey = `${skill.id}:${skill.sourceOrigin}:${projectRoot ?? ""}`;
    const disabledByDefault = skill.state === "disabled_by_policy";
    const toggledOn = skillToggleStates[skillKey] ?? !disabledByDefault;
    const cardSkill: AwakenSkillItem = {
      id: skill.id,
      name: skillName,
      description: skill.whenToUse || skill.description,
      enabled: toggledOn,
      scope:
        skill.sourceOrigin === "bundled"
          ? "awaken"
          : skill.sourceOrigin === "project"
            ? "project"
            : skill.sourceOrigin === "user"
              ? "global"
              : skill.sourceOrigin === "agents"
                ? "agents"
                : skill.sourceOrigin === "mcp"
                  ? "mcp"
                  : "other",
      sourceLabel: skillSourceLabel(skill.sourceOrigin, skill.id),
      diagnostic:
        diagnostics.length > 0 ? `诊断：${diagnostics.join(" · ")}` : undefined,
      previewable,
      busy: busySkillId === skill.id,
      toggleDisabled: busySkillId === skill.id,
      projectId: projectRoot,
    };
    return (
      <AwakenSkillCard
        key={skillKey}
        skill={cardSkill}
        onPreview={previewable ? () => openSkillPreview(skill, projectRoot) : undefined}
        onEnabledChange={(enabled) => {
          void handleToggleSkill(skill, skillKey, enabled);
        }}
      />
    );
  }

  function projectSkillCountText(group: VisibleSkillProjectGroup) {
    if (group.errorMessage) {
      return "读取失败";
    }
    if (normalizedQuery && group.visibleSkills.length !== group.skills.length) {
      return `${group.visibleSkills.length}/${group.skills.length} Skill`;
    }
    return `${group.skills.length} Skill`;
  }

  function renderProjectSkillFolder(group: VisibleSkillProjectGroup) {
    const open = openProjectRoot === group.id;
    return (
      <AwakenFolder
        key={group.id}
        label={group.label}
        meta={projectSkillCountText(group)}
        size="dense"
        open={open}
        showNativeTitle={false}
        aria-label={`打开${group.label} 项目 Skills`}
        onOpen={() => setOpenProjectRoot(group.id)}
      />
    );
  }

  const builtInMcpConnectorItems = visibleAwakenBuiltInMcpConnectors.map(
    (connector) =>
      mcpConnectorCardItem(
        connector,
        busyConnectorId,
        findConnectorOwnedMcpServer(connector, mcpServers),
        busyMcpServerName,
        Boolean(connectorAuthSessions[connector.id]),
      ),
  );
  const userMcpConnectorItems = [
    ...visibleUserInstalledMcpConnectors.map((connector) =>
      mcpConnectorCardItem(
        connector,
        busyConnectorId,
        findConnectorOwnedMcpServer(connector, mcpServers),
        busyMcpServerName,
        Boolean(connectorAuthSessions[connector.id]),
      ),
    ),
    ...visibleMcpServers.map((server) => mcpServerCardItem(server, busyMcpServerName)),
  ];
  const cliConnectorItems = visibleCliConnectors
    .map((connector) => cliConnectorCardItem(connector, cliConnectors, busyConnectorId))
    .filter((item): item is AwakenConnectorCardItem => item !== null);
  const selectedCliConnector = connectors.find((entry) => entry.id === cliDetailConnectorId) ?? null;
  const selectedCliView = cliConnectors.find((entry) => entry.providerId === cliDetailConnectorId);
  const selectedCliReadiness = selectedCliView?.readiness ?? selectedCliConnector?.cliReadiness ?? null;
  const selectedCliProduct = selectedCliConnector
    ? cliConnectorProductCopy(selectedCliConnector.id)
    : null;

  function pluginInstallSource(path: string): RuntimePluginSource {
    return { kind: "local_path", path: path.trim() };
  }

  async function handleValidatePlugin(path: string): Promise<void> {
    const generation = pluginValidationGenerationRef.current + 1;
    pluginValidationGenerationRef.current = generation;
    setPluginValidation(null);
    setValidatingPlugin(true);
    setErrorMessage(null);
    try {
      const result = await runtimeClient.validatePlugin(pluginInstallSource(path));
      if (pluginValidationGenerationRef.current !== generation) return;
      setPluginValidation(result);
      if (!result.valid) {
        setErrorMessage("插件 ZIP 格式不符合要求");
      }
    } catch (error) {
      if (pluginValidationGenerationRef.current !== generation) return;
      if (pluginErrorCode(error) === "SKILL_PACKAGE_SELECTED") {
        openRoutedSkillInstaller(path);
        return;
      }
      setErrorMessage(pluginValidationErrorMessage(error));
    } finally {
      if (pluginValidationGenerationRef.current === generation) {
        setValidatingPlugin(false);
      }
    }
  }

  async function handleInstallPlugin(): Promise<void> {
    const expectedDigest = pluginValidation?.digest?.trim();
    if (pluginValidation?.valid !== true || !expectedDigest) {
      setErrorMessage("请先选择格式正确的插件 ZIP");
      return;
    }
    setInstallingPlugin(true);
    setErrorMessage(null);
    try {
      const result = await runtimeClient.installPlugin({
        source: pluginInstallSource(pluginLocalPath),
        replace: true,
        expectedDigest,
      });
      setPluginDialogOpen(false);
      invalidatePluginValidation();
      setActivePluginView("installed");
      setSelectedPlugin(result.plugin);
      setPlugins((current) => [
        ...current.filter((item) => item.name !== result.plugin.name),
        result.plugin,
      ]);
      onNotice("插件已安装，将在新任务中生效");
      await refresh();
    } catch (error) {
      if (pluginErrorCode(error) === "PLUGIN_SOURCE_CHANGED") {
        await handleValidatePlugin(pluginLocalPath);
      } else {
        setErrorMessage(pluginErrorMessage(error));
      }
    } finally {
      setInstallingPlugin(false);
    }
  }

  async function handleTogglePlugin(plugin: RuntimePlugin, enabled: boolean): Promise<void> {
    const optimistic = { ...plugin, enabled };
    setPlugins((current) => current.map((item) => (item.name === plugin.name ? optimistic : item)));
    if (selectedPlugin?.name === plugin.name) {
      setSelectedPlugin(optimistic);
    }
    setBusyPluginName(plugin.name);
    try {
      const next = enabled
        ? await runtimeClient.enablePlugin(plugin.name)
        : await runtimeClient.disablePlugin(plugin.name);
      setPlugins((current) => current.map((item) => (item.name === next.name ? next : item)));
      if (selectedPlugin?.name === next.name) {
        setSelectedPlugin(next);
      }
    } catch (error) {
      setPlugins((current) => current.map((item) => (item.name === plugin.name ? plugin : item)));
      if (selectedPlugin?.name === plugin.name) {
        setSelectedPlugin(plugin);
      }
      setErrorMessage(pluginErrorMessage(error));
    } finally {
      setBusyPluginName(null);
    }
  }

  async function openPluginDetail(name: string): Promise<void> {
    const snapshot = plugins.find((plugin) => plugin.name === name);
    if (snapshot) {
      setSelectedPlugin(snapshot);
    }
    try {
      setSelectedPlugin(await runtimeClient.getPlugin(name));
    } catch (error) {
      const message = pluginErrorMessage(error);
      setErrorMessage(message);
      if (!snapshot) {
        setSelectedPlugin(null);
      }
    }
  }

  if (section === "plugins") {
    if (selectedPlugin) {
      return (
        <>
          <PluginConsole
            plugin={selectedPlugin}
            runtimeClient={runtimeClient}
            busy={busyPluginName === selectedPlugin.name}
            onBack={() => setSelectedPlugin(null)}
            onPluginUpdated={(next) => {
              setSelectedPlugin(next);
              setPlugins((current) =>
                current.map((item) => (item.name === next.name ? next : item)),
              );
            }}
            onUninstall={() => setPendingPluginUninstall(selectedPlugin)}
            onUsePlugin={onUsePlugin}
            onUsePluginComponent={onUsePluginComponent}
            onNotice={onNotice}
          />
          <Dialog
            open={pendingPluginUninstall !== null}
            title="卸载插件"
            onOpenChange={(open) => { if (!open) setPendingPluginUninstall(null); }}
            footer={pendingPluginUninstall ? <>
              <Button variant="ghost" onClick={() => setPendingPluginUninstall(null)}>取消</Button>
              <Button
                variant="primary"
                loading={busyPluginName === pendingPluginUninstall.name}
                onClick={() => {
                  const plugin = pendingPluginUninstall;
                  void (async () => {
                    setBusyPluginName(plugin.name);
                    try {
                      await runtimeClient.uninstallPlugin(plugin.name);
                      setPendingPluginUninstall(null);
                      setSelectedPlugin(null);
                      onNotice("插件已移入回收站");
                      await refresh(false);
                    } catch (error) {
                      setErrorMessage(pluginErrorMessage(error));
                    } finally {
                      setBusyPluginName(null);
                    }
                  })();
                }}
              >确认卸载</Button>
            </> : undefined}
          >
            <p>卸载「{pendingPluginUninstall?.name}」会把插件包和数据目录移入回收站，可从系统回收站恢复。</p>
          </Dialog>
        </>
      );
    }
    return (
      <>
        <AwakenPlugins
          activeView={activePluginView}
          searchValue={query}
          plugins={plugins.map((plugin) => ({
            name: plugin.name,
            displayName: plugin.displayName,
            description: plugin.description,
            dialect: plugin.dialect,
            builtin: plugin.builtin,
            brandIcon: plugin.brandIcon,
            builtinOrder: plugin.builtinOrder,
            enabled: plugin.enabled,
            busy: busyPluginName === plugin.name,
            diagnostic: plugin.diagnostics?.[0]?.message,
          }))}
          loading={loading}
          errorMessage={pluginDialogOpen ? null : errorMessage}
          onSearchChange={setQuery}
          onViewChange={(view) => {
            setActivePluginView(view);
            setQuery("");
          }}
          onRefresh={() => { void refresh(); }}
          onInstall={() => {
            invalidatePluginValidation();
            setPluginLocalPath("");
            setPluginDialogOpen(true);
          }}
          onPluginOpen={(plugin) => { void openPluginDetail(plugin.name); }}
          onPluginEnabledChange={(plugin, enabled) => {
            const runtimePlugin = plugins.find((item) => item.name === plugin.name);
            if (runtimePlugin) void handleTogglePlugin(runtimePlugin, enabled);
          }}
        />
        <AwakenPluginInstallDialog
          open={pluginDialogOpen}
          localPath={pluginLocalPath}
          validation={pluginValidation}
          validating={validatingPlugin}
          errorMessage={pluginDialogOpen ? errorMessage : null}
          busy={installingPlugin}
          onOpenChange={(open) => {
            setPluginDialogOpen(open);
            if (!open) invalidatePluginValidation();
          }}
          onPickLocal={() => { void handlePickPluginSource(); }}
          onDropFile={(file) => { void handleDropPluginSource(file); }}
          onSubmit={() => { void handleInstallPlugin(); }}
        />
      </>
    );
  }

  if (section === "connectors") {
    return (
      <>
        <AwakenConnectors
          activeKind={activeConnectorKind}
          searchValue={query}
          builtInMcpConnectors={builtInMcpConnectorItems}
          userMcpConnectors={userMcpConnectorItems}
          cliConnectors={cliConnectorItems}
          loading={loading}
          errorMessage={errorMessage}
          onKindChange={setActiveConnectorKind}
          onSearchChange={setQuery}
          onRefresh={() => { void refresh(); }}
          onConfigureMcp={() => {
            setErrorMessage(null);
            setMcpDialogOpen(true);
          }}
          onConnectorAction={(item) => {
            if (item.variant === "cli") {
              void openCliConnectorDetail(item.id as CliProviderId);
              return;
            }
            const connector = connectors.find((entry) => entry.id === item.id);
            if (connector) void handleConnectorAction(connector);
          }}
          onConnectorAdminApprovalRequired={(item) => {
            const connector = connectors.find((entry) => entry.id === item.id);
            if (connector) void handleConnectorAdminApprovalRequired(connector);
          }}
          onMcpServerEnabledChange={(item) => {
            const serverName = item.mcpServerName ?? item.id;
            const server = mcpServers.find((entry) => entry.name === serverName);
            if (server) void handleToggleMcpServer(server);
          }}
          onMcpServerSettings={(item) => {
            const serverName = item.mcpServerName ?? item.id;
            const server = mcpServers.find((entry) => entry.name === serverName);
            if (server) void loadMcpSettings(server);
          }}
        />

        <AwakenCliConnectorDetailDialog
          open={cliDetailConnectorId !== null}
          providerId={selectedCliConnector?.id ?? cliDetailConnectorId ?? ""}
          displayName={selectedCliProduct?.displayName || selectedCliConnector?.displayName || "CLI"}
          description={selectedCliProduct?.description || selectedCliConnector?.description?.trim() || "官方 CLI 连接器"}
          readiness={selectedCliReadiness ? {
            install: selectedCliReadiness.install,
            executable: selectedCliReadiness.executable,
            authentication: selectedCliReadiness.authentication,
            account: selectedCliReadiness.account,
            safeErrorMessage: selectedCliReadiness.safeError?.message,
          } : null}
          authSession={selectedCliView?.authSession ? { phase: selectedCliView.authSession.phase } : undefined}
          installProgress={selectedCliView?.installProgress ? {
            percent: selectedCliView.installProgress.percent,
            label: selectedCliView.installProgress.label,
          } : undefined}
          brandLogoSrc={selectedCliConnector ? connectorBrandLogo(selectedCliConnector.id) : undefined}
          brandTone={connectorBrandTone(selectedCliConnector?.id ?? "")}
          loadingProfile={cliDetailLoading}
          busy={cliDetailConnectorId !== null && busyConnectorId === cliDetailConnectorId}
          onOpenChange={(open) => {
            if (!open) {
              setCliDetailConnectorId(null);
              setCliDetailLoading(false);
            }
          }}
          onAction={(action) => {
            if (cliDetailConnectorId) handleCliDialogAction(cliDetailConnectorId, action);
          }}
        />

        <AwakenMcpConfigDialog
          open={mcpDialogOpen}
          mode={mcpConfigMode}
          value={mcpConfigText}
          errorMessage={mcpDialogOpen ? errorMessage : null}
          busy={configuringMcp}
          onOpenChange={setMcpDialogOpen}
          onModeChange={handleMcpConfigModeChange}
          onValueChange={handleMcpConfigTextChange}
          onSubmit={() => { void handleConfigureMcp(); }}
        />

        <AwakenMcpSettingsDialog
          open={mcpSettingsOpen}
          serverName={selectedMcpServer?.displayName?.trim() || selectedMcpServer?.name}
          scopeLabel={mcpSettingsScope ? sourceLabel(mcpSettingsScope) : ""}
          configValue={mcpSettingsConfigText}
          tools={mcpTools}
          logs={mcpLogs}
          errorMessage={mcpSettingsError}
          loading={loadingMcpSettings}
          saving={savingMcpConfig}
          onOpenChange={(open) => {
            setMcpSettingsOpen(open);
            if (!open) {
              setSelectedMcpServer(null);
              setMcpSettingsConfigText("");
              setMcpSettingsScope("");
              setMcpSettingsError(null);
              setMcpTools([]);
              setMcpLogs([]);
            }
          }}
          onConfigValueChange={setMcpSettingsConfigText}
          onDelete={() => { void handleDeleteMcpServer(); }}
          onSave={() => { void handleSaveMcpConfig(); }}
        />
      </>
    );
  }

  const title = "技能";
  const subtitle = "管理 Awaken、项目与全局 Skills，扩展 Awaken 的执行能力。";

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
              <AwakenIcon concept="refresh" size={16} />
            </button>
            <Button
              variant="primary"
              size="md"
              onClick={() => {
                setSkillInstallScope("project");
                setSkillInstallProjectRoot(skillProjectRoot);
                setSkillFile(null);
                setSkillSourcePath("");
                setSkillDialogOpen(true);
              }}
            >
              <AwakenIcon concept="import" size={16} />
              安装 Skill
            </Button>
          </div>
        </header>

        <div className="extension-toolbar extension-toolbar-with-tabs">
          <div className="extension-category-tabs" role="tablist" aria-label="技能视图">
              {SKILL_VIEW_TABS.map((tab) => (
                <button
                  key={tab.id}
                  type="button"
                  className={`extension-category-tab${
                    activeSkillView === tab.id
                      ? " extension-category-tab-active"
                      : ""
                  }`}
                  role="tab"
                  aria-selected={activeSkillView === tab.id}
                  onClick={() => setActiveSkillView(tab.id as SkillView)}
                >
                  {tab.label}
                </button>
              ))}
          </div>
          <input
            value={query}
            placeholder="搜索..."
            onChange={(event) => setQuery(event.target.value)}
          />
        </div>

        {errorMessage ? <div className="extension-message" role="alert">{errorMessage}</div> : null}

        <div className="extension-sections">
            {activeSkillView === "awaken" ? (
              <section className="extension-section" aria-label="Awaken Skills">
                <div className="extension-grid">
                  {visibleAwakenSkills.length > 0 ? (
                    visibleAwakenSkills.map((skill) => renderSkillCard(skill))
                  ) : (
                    <div className="extension-empty">
                      {normalizedQuery
                        ? "未找到匹配的 Skill"
                        : "内置 Skills 未就绪，重启 Awaken 后重试"}
                    </div>
                  )}
                </div>
              </section>
            ) : activeSkillView === "project" ? (
              <section className="extension-section" aria-label="项目 Skills">
                {visibleProjectSkillGroups.length > 0 ? (
                  <div className="skill-project-folder-grid">
                    {visibleProjectSkillGroups.map(renderProjectSkillFolder)}
                  </div>
                ) : (
                  <div className="extension-empty">
                    {normalizedQuery ? "未找到匹配的 Skill" : "暂无项目 Skills"}
                  </div>
                )}
              </section>
            ) : activeSkillView === "agents" ? (
              <section className="extension-section" aria-label="Agents Skills">
                <div className="extension-grid">
                  {visibleAgentsSkills.length > 0 ? (
                    visibleAgentsSkills.map(({ skill, projectRoot }) =>
                      renderSkillCard(skill, projectRoot),
                    )
                  ) : (
                    <div className="extension-empty">
                      {normalizedQuery ? "未找到匹配的 Skill" : "暂无 Agents Skills"}
                    </div>
                  )}
                </div>
              </section>
            ) : (
              <>
                <section className="extension-section" aria-label="全局 Skills">
                  <div className="extension-grid">
                    {visibleUserSkills.length > 0 ? (
                      visibleUserSkills.map((skill) => renderSkillCard(skill))
                    ) : (
                      <div className="extension-empty">
                        {normalizedQuery ? "未找到匹配的 Skill" : "暂无全局 Skills"}
                      </div>
                    )}
                  </div>
                </section>
                {visibleMcpSkills.length > 0 ? (
                  <section className="extension-section">
                    <h2>MCP 动态</h2>
                    <div className="extension-grid">
                      {visibleMcpSkills.map((skill) => renderSkillCard(skill))}
                    </div>
                  </section>
                ) : null}
                {visibleOtherSkills.length > 0 ? (
                  <section className="extension-section">
                    <h2>其他来源</h2>
                    <div className="extension-grid">
                      {visibleOtherSkills.map((skill) => renderSkillCard(skill))}
                    </div>
                  </section>
                ) : null}
              </>
            )}
        </div>
      </div>

      <AwakenProjectSkillsDialog
        open={openProjectSkillDialog !== null}
        project={openProjectSkillDialog}
        detailView={projectSkillPreviewTarget && projectSkillPreviewSkill ? {
          mode: "preview",
          title: `${projectSkillPreviewSkill.displayName || projectSkillPreviewSkill.name} 的 Skill 包`,
          titleVisuallyHidden: true,
          initialFocusRef: projectSkillPreviewScrollRef,
          content: (
            <SkillPreviewContent
              target={projectSkillPreviewTarget}
              presentation={createSkillPreviewPresentation(
                projectSkillPreviewSkill,
                projectSkillPreviewTarget.projectRoot,
              )}
              runtimeClient={runtimeClient}
              scrollRef={projectSkillPreviewScrollRef}
              onEnabledChange={(enabled) => {
                const projectRoot = projectSkillPreviewTarget.projectRoot;
                if (!projectRoot) {
                  return;
                }
                const skillKey = `${projectSkillPreviewSkill.id}:${projectSkillPreviewSkill.sourceOrigin}:${projectRoot}`;
                void handleToggleSkill(projectSkillPreviewSkill, skillKey, enabled);
              }}
            />
          ),
          footer: isSkillPackageDeletable(projectSkillPreviewSkill) ? (
            <SkillPreviewDeleteButton
              deleting={deletingSkill}
              onDelete={() => {
                const projectRoot = projectSkillPreviewTarget.projectRoot;
                setSkillPreviewTarget(null);
                requestSkillDelete(projectSkillPreviewSkill, projectRoot);
              }}
            />
          ) : undefined,
          returnFocusSkillId: projectSkillPreviewTarget.skillId,
        } : projectSkillDeleteTarget ? {
          mode: "delete",
          title: "删除 Skill",
          content: (
            <div className="extension-dialog-body skill-delete-dialog-body">
              <p>
                确定要删除 Skill「
                <strong>
                  {projectSkillDeleteTarget.skill.displayName
                    || projectSkillDeleteTarget.skill.name}
                </strong>
                」吗？
              </p>
              <p className="extension-dialog-note">
                确认后会立即从对应的 .awaken/skills 目录移除，并将完整 Skill 包移动到系统废纸篓。
              </p>
              {skillDeleteError ? (
                <div className="extension-message" role="alert">{skillDeleteError}</div>
              ) : null}
            </div>
          ),
          footer: (
            <>
              <Button
                autoFocus
                variant="default"
                size="md"
                disabled={deletingSkill}
                onClick={cancelSkillDelete}
              >
                取消
              </Button>
              <Button
                className="skill-delete-confirm-button"
                variant="primary"
                size="md"
                loading={deletingSkill}
                onClick={() => {
                  void handleDeleteSkill();
                }}
              >
                <AwakenIcon concept="delete" size={16} />
                确认删除
              </Button>
            </>
          ),
          returnFocusSkillId: skillCatalogIdentity(
            projectSkillDeleteTarget.skill,
            projectSkillDeleteTarget.projectRoot,
          ),
        } : null}
        onDetailClose={() => {
          if (projectSkillPreviewTarget) {
            setSkillPreviewTarget(null);
          } else if (projectSkillDeleteTarget && !deletingSkill) {
            cancelSkillDelete();
          }
        }}
        onOpenChange={(open) => {
          if (!open) {
            setOpenProjectRoot(null);
          }
        }}
        onSkillPreview={(item) => {
          const group = openProjectSkillGroup;
          const skill = group?.skills.find((entry) =>
            skillCatalogIdentity(entry, entry.projectPath || group.root) === item.id,
          );
          if (group && skill) {
            const projectRoot = skill.projectPath || group.root;
            openSkillPreview(skill, projectRoot, item.id);
          }
        }}
        onSkillToggle={(item, enabled) => {
          const group = openProjectSkillGroup;
          const skill = group?.skills.find((entry) =>
            skillCatalogIdentity(entry, entry.projectPath || group.root) === item.id,
          );
          if (group && skill) {
            const projectRoot = skill.projectPath || group.root;
            const skillKey = `${skill.id}:${skill.sourceOrigin}:${projectRoot}`;
            void handleToggleSkill(skill, skillKey, enabled);
          }
        }}
      />

      <SkillPreviewDialog
        target={skillPreviewTarget?.projectRoot ? null : skillPreviewTarget}
        presentation={standaloneSkillPreviewSkill
          ? createSkillPreviewPresentation(standaloneSkillPreviewSkill)
          : null}
        runtimeClient={runtimeClient}
        onEnabledChange={standaloneSkillPreviewSkill ? (enabled) => {
          const skillKey = `${standaloneSkillPreviewSkill.id}:${standaloneSkillPreviewSkill.sourceOrigin}:`;
          void handleToggleSkill(standaloneSkillPreviewSkill, skillKey, enabled);
        } : undefined}
        onDelete={standaloneSkillPreviewSkill && isSkillPackageDeletable(standaloneSkillPreviewSkill)
          ? () => {
              setSkillPreviewTarget(null);
              requestSkillDelete(standaloneSkillPreviewSkill);
            }
          : undefined}
        onOpenChange={(open) => {
          if (!open) {
            setSkillPreviewTarget(null);
          }
        }}
      />

      <Dialog
        open={pendingSkillDelete !== null && !pendingSkillDelete.projectRoot}
        title="删除 Skill"
        onOpenChange={(open) => {
          if (!open && !deletingSkill) {
            cancelSkillDelete();
          }
        }}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={deletingSkill}
              onClick={cancelSkillDelete}
            >
              取消
            </Button>
            <Button
              className="skill-delete-confirm-button"
              variant="primary"
              size="md"
              loading={deletingSkill}
              onClick={() => {
                void handleDeleteSkill();
              }}
            >
              <AwakenIcon concept="delete" size={16} />
              确认删除
            </Button>
          </>
        }
      >
        <div className="extension-dialog-body skill-delete-dialog-body">
          <p>
            确定要删除 Skill「
            <strong>
              {pendingSkillDelete?.skill.displayName || pendingSkillDelete?.skill.name}
            </strong>
            」吗？
          </p>
          <p className="extension-dialog-note">
            确认后会立即从对应的 .awaken/skills 目录移除，并将完整 Skill 包移动到系统废纸篓。
          </p>
          {skillDeleteError ? (
            <div className="extension-message" role="alert">{skillDeleteError}</div>
          ) : null}
        </div>
      </Dialog>

      <Dialog
        open={skillDialogOpen}
        title="安装 Skill"
        onOpenChange={(open) => {
          setSkillDialogOpen(open);
          if (!open) {
            skillPackageRoutingGenerationRef.current += 1;
            setClassifyingSkillPackage(false);
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
              disabled={(!skillFile && !skillSourcePath) || classifyingSkillPackage}
              onClick={() => {
                void handleInstallSkill();
              }}
            >
              <AwakenIcon concept="step-done" size={16} />
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
                  <AwakenIcon concept="folder-expanded" size={16} />
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
              void handleSkillFileSelected(event.dataTransfer.files.item(0));
            }}
          >
            <input
              type="file"
              accept=".md,.zip"
              onChange={(event) => {
                void handleSkillFileSelected(event.target.files?.item(0) ?? null);
              }}
            />
            <AwakenIcon concept="import" size={24} />
            <strong>
              {classifyingSkillPackage
                ? "正在识别安装包…"
                : skillFile?.name || fileNameFromLocalPath(skillSourcePath) || "拖放或点击上传"}
            </strong>
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
