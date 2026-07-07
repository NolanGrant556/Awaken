import { useEffect, useMemo, useState } from "react";
import {
  Check,
  FileUp,
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
    case "bundled":
      return "内置";
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

function matchesQuery(value: string | undefined, query: string): boolean {
  if (!query) return true;
  return (value ?? "").toLowerCase().includes(query);
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
  const [query, setQuery] = useState("");
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
  const [mcpSettingsOpen, setMcpSettingsOpen] = useState(false);
  const [selectedMcpServer, setSelectedMcpServer] = useState<RuntimeMcpServer | null>(null);
  const [mcpSettingsConfigText, setMcpSettingsConfigText] = useState("");
  const [mcpSettingsScope, setMcpSettingsScope] = useState("");
  const [mcpTools, setMcpTools] = useState<McpToolInfo[]>([]);
  const [mcpLogs, setMcpLogs] = useState<RuntimeLogEntry[]>([]);
  const [savingMcpConfig, setSavingMcpConfig] = useState(false);
  const [loadingMcpSettings, setLoadingMcpSettings] = useState(false);

  const normalizedQuery = query.trim().toLowerCase();

  async function refresh(showLoading = true): Promise<void> {
    if (!desktopReady) {
      setErrorMessage("Awaken runtime 未启动");
      return;
    }
    if (showLoading) {
      setLoading(true);
    }
    setErrorMessage(null);
    try {
      const [nextConnectors, nextMcpServers, nextSkills] = await Promise.all([
        runtimeClient.listConnectors(),
        runtimeClient.listMcpServers(),
        runtimeClient.listSkills(),
      ]);
      setConnectors(nextConnectors);
      setMcpServers(nextMcpServers);
      setSkills(nextSkills);
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
      skills.filter(
        (skill) =>
          matchesQuery(skill.displayName, normalizedQuery) ||
          matchesQuery(skill.name, normalizedQuery) ||
          matchesQuery(skill.description, normalizedQuery) ||
          matchesQuery(skill.whenToUse, normalizedQuery) ||
          matchesQuery(skill.id, normalizedQuery),
      ),
    [skills, normalizedQuery],
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

  async function handleInstallSkill(): Promise<void> {
    if (!skillFile) {
      setErrorMessage("请选择 SKILL.md、.skill 或 zip 文件");
      return;
    }
    setInstallingSkill(true);
    setErrorMessage(null);
    try {
      const contentBase64 = await fileToBase64(skillFile);
      const result = await runtimeClient.installSkill({
        fileName: skillFile.name,
        contentBase64,
      });
      setSkillDialogOpen(false);
      setSkillFile(null);
      onNotice(`已安装 ${result.skill.displayName || result.skill.name}`);
      await refresh();
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "Skill 安装失败");
    } finally {
      setInstallingSkill(false);
    }
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
            <span>{sourceLabel(skill.sourceOrigin)}</span>
            <span>{skill.context}</span>
            {skill.requiredMcpServers?.length ? (
              <span>MCP: {skill.requiredMcpServers.join(", ")}</span>
            ) : null}
          </div>
        </div>
      </article>
    );
  }

  const title = section === "connectors" ? "连接器" : "技能";
  const subtitle =
    section === "connectors"
      ? "配置 MCP servers，连接外部系统和工具。"
      : "安装和管理项目级 Skill，扩展 Awaken 的执行能力。";
  const primaryAction =
    section === "connectors"
      ? { label: "配置 MCP", icon: <Plus size={16} strokeWidth={1.9} /> }
      : { label: "安装 Skill", icon: <Upload size={16} strokeWidth={1.9} /> };

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
            <Button
              variant="primary"
              size="md"
              onClick={() => {
                if (section === "connectors") {
                  setMcpDialogOpen(true);
                } else {
                  setSkillDialogOpen(true);
                }
              }}
            >
              {primaryAction.icon}
              {primaryAction.label}
            </Button>
          </div>
        </header>

        <div className="extension-toolbar">
          <input
            value={query}
            placeholder="搜索..."
            onChange={(event) => setQuery(event.target.value)}
          />
        </div>

        {errorMessage ? <div className="extension-message" role="alert">{errorMessage}</div> : null}

        {section === "connectors" ? (
          <div className="extension-sections">
            <section className="extension-section">
              <h2>市场</h2>
              <div className="extension-grid">
                {visibleConnectors.length > 0 ? (
                  visibleConnectors.map(renderConnectorCard)
                ) : (
                  <div className="extension-empty">暂无连接器</div>
                )}
              </div>
            </section>
            <section className="extension-section">
              <h2>MCP Servers</h2>
              <div className="extension-grid">
                {visibleMcpServers.length > 0 ? (
                  visibleMcpServers.map(renderMcpServerCard)
                ) : (
                  <div className="extension-empty">还没有配置 MCP server</div>
                )}
              </div>
            </section>
          </div>
        ) : (
          <section className="extension-section">
            <h2>已安装</h2>
            <div className="extension-grid">
              {visibleSkills.length > 0 ? (
                visibleSkills.map(renderSkillCard)
              ) : (
                <div className="extension-empty">暂无 Skill</div>
              )}
            </div>
          </section>
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
              accept=".md,.skill,.zip"
              onChange={(event) => setSkillFile(event.target.files?.item(0) ?? null)}
            />
            <FileUp size={22} strokeWidth={1.9} />
            <strong>{skillFile ? skillFile.name : "拖放或点击上传"}</strong>
          </label>
          <p>支持 SKILL.md、.skill，或包含 SKILL.md 的 zip 包。</p>
        </div>
      </Dialog>
    </section>
  );
}
