import "./AwakenConnectors.demo.css";
import { useEffect, useMemo, useRef, useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenCliConnectorDetailDialog } from "../AwakenCliConnectorDetailDialog";
import { AwakenConnectorBrandIcon } from "../AwakenConnectorBrandIcon";
import { AwakenConnectorCard } from "../AwakenConnectorCard";
import { AwakenSwitch } from "../AwakenSwitch";
import { AwakenMcpConfigDialog } from "../AwakenMcpConfigDialog";
import { AwakenMcpSettingsDialog } from "../AwakenMcpSettingsDialog";
import { AwakenConnectors } from "./AwakenConnectors";
import type { AwakenConnectorCardItem, AwakenConnectorKind } from "./AwakenConnectors.types";
import {
  AwakenBuiltInMcpConnectorStates,
  AwakenCliConnectedState,
  AwakenCliConnectorStates,
  AwakenCliInstalledLoggedOutState,
  AwakenMcpConfigExample,
  AwakenMcpLogStates,
  AwakenMcpToolStates,
  AwakenUserMcpConnectorStates,
} from "./AwakenConnectors.states";

function includesQuery(item: AwakenConnectorCardItem, query: string): boolean {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return true;
  return [item.displayName, item.description, item.id, ...(item.tags ?? [])]
    .some((value) => value.toLowerCase().includes(normalized));
}

export function AwakenConnectorsDemo() {
  const [activeKind, setActiveKind] = useState<AwakenConnectorKind>("mcp");
  const [searchValue, setSearchValue] = useState("");
  const [theme, setTheme] = useState<"light" | "dark">("light");
  const [connectedById, setConnectedById] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(AwakenBuiltInMcpConnectorStates.map((item) => [item.id, item.connected ?? false])),
  );
  const [enabledById, setEnabledById] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(AwakenUserMcpConnectorStates.map((item) => [item.id, item.enabled ?? false])),
  );
  const [configOpen, setConfigOpen] = useState(false);
  const [configValue, setConfigValue] = useState(AwakenMcpConfigExample);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsServerName, setSettingsServerName] = useState("playwright");
  const [cliDialog, setCliDialog] = useState<"dingtalk-workspace" | "lark-cli" | null>(null);
  const [cliConnectedById, setCliConnectedById] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(AwakenCliConnectorStates.map((item) => [item.id, item.connected ?? false])),
  );
  const originalThemeRef = useRef<string | null>(null);

  useEffect(() => {
    const root = document.documentElement;
    if (originalThemeRef.current === null) originalThemeRef.current = root.getAttribute("data-aw-theme") ?? "";
    root.setAttribute("data-aw-theme", theme);
  }, [theme]);

  useEffect(() => () => {
    if (originalThemeRef.current) document.documentElement.setAttribute("data-aw-theme", originalThemeRef.current);
    else document.documentElement.removeAttribute("data-aw-theme");
  }, []);

  const builtIn = useMemo(() => AwakenBuiltInMcpConnectorStates
    .map((item) => ({
      ...item,
      connected: connectedById[item.id],
      state: connectedById[item.id] ? "connected" as const : "disconnected" as const,
      stateLabel: connectedById[item.id] ? "已连接" : "未连接",
    }))
    .filter((item) => includesQuery(item, searchValue)), [connectedById, searchValue]);
  const userMcp = useMemo(() => AwakenUserMcpConnectorStates
    .map((item) => ({ ...item, enabled: enabledById[item.id] }))
    .filter((item) => includesQuery(item, searchValue)), [enabledById, searchValue]);
  const cli = useMemo(() => AwakenCliConnectorStates
    .map((item) => ({ ...item, connected: cliConnectedById[item.id] ?? false }))
    .filter((item) => includesQuery(item, searchValue)), [cliConnectedById, searchValue]);
  const activeCliItem = AwakenCliConnectorStates.find((item) => item.id === cliDialog);
  const activeCliConnected = cliDialog ? cliConnectedById[cliDialog] ?? false : false;
  const sampleCatalog = builtIn[0] ?? AwakenBuiltInMcpConnectorStates[0];
  const sampleServer = userMcp[0] ?? AwakenUserMcpConnectorStates[0];

  function handlePrimaryAction(item: AwakenConnectorCardItem): void {
    if (item.variant === "cli") {
      setCliDialog(item.id as "dingtalk-workspace" | "lark-cli");
      return;
    }
    setConnectedById((current) => ({ ...current, [item.id]: !current[item.id] }));
  }

  return (
    <section className="ads-component-demo aw-connectors-demo" aria-label="「连接器」组件库 AwakenConnectors 演示">
      <header className="ads-component-demo-header">
        <h2>「连接器」组件库 <code>AwakenConnectors</code></h2>
        <p>先查看完整 MCP/CLI 组合，再逐个操作连接器独有的小组件与状态弹窗。</p>
      </header>

      <div className="aw-connectors-demo__controls">
        <p>组合组件与下面的小组件共享模拟状态；所有动作无运行时副作用。</p>
        <div>
          <AwakenButton size="sm" variant={theme === "light" ? "default" : "ghost"} aria-pressed={theme === "light"} onClick={() => setTheme("light")}>浅色</AwakenButton>
          <AwakenButton size="sm" variant={theme === "dark" ? "default" : "ghost"} aria-pressed={theme === "dark"} onClick={() => setTheme("dark")}>暗色</AwakenButton>
        </div>
      </div>

      <section className="aw-connectors-demo__composition">
        <header><h3>完整组合</h3><p><code>AwakenConnectors</code> 组合标题、刷新、配置入口、MCP/CLI、搜索、分组和卡片。</p></header>
        <div className="aw-connectors-demo__composition-canvas">
          <AwakenConnectors
            activeKind={activeKind}
            searchValue={searchValue}
            builtInMcpConnectors={builtIn}
            userMcpConnectors={userMcp}
            cliConnectors={cli}
            onKindChange={(kind) => { setActiveKind(kind); setSearchValue(""); }}
            onSearchChange={setSearchValue}
            onRefresh={() => undefined}
            onConfigureMcp={() => setConfigOpen(true)}
            onConnectorAction={handlePrimaryAction}
            onMcpServerEnabledChange={(item, enabled) => setEnabledById((current) => ({ ...current, [item.id]: enabled }))}
            onMcpServerSettings={(item) => {
              setSettingsServerName(item.displayName);
              setSettingsOpen(true);
            }}
          />
        </div>
      </section>

      <section className="aw-connectors-demo__parts">
        <header><h3>可独立组合的小组件</h3><p>连接器当前独有的卡片、品牌、开关与三个弹窗均已进入组件库。</p></header>
        <div className="aw-connectors-demo__part-list">
          <article><header><h4>连接器卡片</h4><code>AwakenConnectorCard</code><p>MCP catalog、用户 server 与 CLI 共用稳定卡片契约。</p></header><div className="aw-connectors-demo__part-canvas aw-connectors-demo__card-stack">
            <AwakenConnectorCard item={sampleCatalog} onPrimaryAction={handlePrimaryAction} />
            <AwakenConnectorCard
              item={sampleServer}
              onEnabledChange={(item, enabled) => setEnabledById((current) => ({ ...current, [item.id]: enabled }))}
              onSettings={(item) => {
                setSettingsServerName(item.displayName);
                setSettingsOpen(true);
              }}
            />
          </div></article>
          <article><header><h4>品牌图标</h4><code>AwakenConnectorBrandIcon</code><p>支持 consumer 提供真实品牌资源，也提供安全 fallback。</p></header><div className="aw-connectors-demo__part-canvas aw-connectors-demo__inline">
            <AwakenConnectorBrandIcon providerId="dingtalk-workspace" label="钉钉" tone="dingtalk" />
            <AwakenConnectorBrandIcon providerId="lark-cli" label="飞书" tone="lark" />
          </div></article>
          <article><header><h4>连接器开关</h4><code>AwakenSwitch</code><p>受控表达用户 MCP server 的启停与忙碌状态。</p></header><div className="aw-connectors-demo__part-canvas aw-connectors-demo__inline">
            <AwakenSwitch checked={enabledById["puppeteer-user"] ?? false} label="puppeteer" onCheckedChange={(enabled) => setEnabledById((current) => ({ ...current, "puppeteer-user": enabled }))} />
            <span>{enabledById["puppeteer-user"] ? "已启用" : "已停用"}</span>
          </div></article>
          <article><header><h4>CLI 详情窗口</h4><code>AwakenCliConnectorDetailDialog</code><p>覆盖安装、授权、进度、账户与退出登录。</p></header><div className="aw-connectors-demo__part-canvas"><AwakenButton onClick={() => setCliDialog("dingtalk-workspace")}>打开 CLI 详情</AwakenButton></div></article>
          <article><header><h4>MCP 配置窗口</h4><code>AwakenMcpConfigDialog</code><p>配置 JSON、风险说明和提交状态。</p></header><div className="aw-connectors-demo__part-canvas"><AwakenButton onClick={() => setConfigOpen(true)}>打开 MCP 配置</AwakenButton></div></article>
          <article><header><h4>MCP 设置窗口</h4><code>AwakenMcpSettingsDialog</code><p>配置、方法、日志、保存与删除动作。</p></header><div className="aw-connectors-demo__part-canvas"><AwakenButton onClick={() => { setSettingsServerName("playwright"); setSettingsOpen(true); }}>打开 MCP 设置</AwakenButton></div></article>
        </div>
      </section>

      <AwakenCliConnectorDetailDialog
        open={cliDialog !== null}
        providerId={activeCliItem?.id ?? "dingtalk-workspace"}
        displayName={activeCliItem?.displayName ?? "钉钉"}
        description={activeCliItem?.description ?? "连接钉钉"}
        readiness={activeCliConnected ? AwakenCliConnectedState : AwakenCliInstalledLoggedOutState}
        brandTone={activeCliItem?.brandTone}
        onOpenChange={(open) => { if (!open) setCliDialog(null); }}
        onAction={(action) => {
          if (!cliDialog) return;
          if (action === "logout") {
            setCliConnectedById((current) => ({ ...current, [cliDialog]: false }));
          } else if (action === "retry_authorization" || action === "authorize") {
            setCliConnectedById((current) => ({ ...current, [cliDialog]: true }));
          }
        }}
      />
      <AwakenMcpConfigDialog open={configOpen} value={configValue} onOpenChange={setConfigOpen} onValueChange={setConfigValue} onSubmit={() => setConfigOpen(false)} />
      <AwakenMcpSettingsDialog
        open={settingsOpen}
        serverName={settingsServerName}
        scopeLabel="用户"
        configValue={configValue}
        tools={AwakenMcpToolStates}
        logs={AwakenMcpLogStates}
        onOpenChange={setSettingsOpen}
        onConfigValueChange={setConfigValue}
        onDelete={() => setSettingsOpen(false)}
        onSave={() => setSettingsOpen(false)}
      />
    </section>
  );
}
