import "./AwakenConnectors.css";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenButton } from "../AwakenButton";
import { AwakenConnectorCard } from "../AwakenConnectorCard";
import type { AwakenConnectorCardItem, AwakenConnectorKind } from "./AwakenConnectors.types";

export type AwakenConnectorsProps = {
  activeKind: AwakenConnectorKind;
  searchValue: string;
  builtInMcpConnectors: AwakenConnectorCardItem[];
  userMcpConnectors: AwakenConnectorCardItem[];
  cliConnectors: AwakenConnectorCardItem[];
  loading?: boolean;
  errorMessage?: string | null;
  onKindChange: (kind: AwakenConnectorKind) => void;
  onSearchChange: (value: string) => void;
  onRefresh: () => void;
  onConfigureMcp: () => void;
  onConnectorAction: (item: AwakenConnectorCardItem) => void;
  onMcpServerEnabledChange: (item: AwakenConnectorCardItem, enabled: boolean) => void;
  onMcpServerSettings: (item: AwakenConnectorCardItem) => void;
};

function ConnectorGrid({
  items,
  emptyMessage,
  onConnectorAction,
  onMcpServerEnabledChange,
  onMcpServerSettings,
}: {
  items: AwakenConnectorCardItem[];
  emptyMessage: string;
  onConnectorAction: AwakenConnectorsProps["onConnectorAction"];
  onMcpServerEnabledChange: AwakenConnectorsProps["onMcpServerEnabledChange"];
  onMcpServerSettings: AwakenConnectorsProps["onMcpServerSettings"];
}) {
  return items.length > 0 ? (
    <div className="aw-connectors__grid">
      {items.map((item) => (
        <AwakenConnectorCard
          key={`${item.variant}:${item.id}`}
          item={item}
          onPrimaryAction={onConnectorAction}
          onEnabledChange={onMcpServerEnabledChange}
          onSettings={onMcpServerSettings}
        />
      ))}
    </div>
  ) : <div className="aw-connectors__empty">{emptyMessage}</div>;
}

export function AwakenConnectors({
  activeKind,
  searchValue,
  builtInMcpConnectors,
  userMcpConnectors,
  cliConnectors,
  loading = false,
  errorMessage,
  onKindChange,
  onSearchChange,
  onRefresh,
  onConfigureMcp,
  onConnectorAction,
  onMcpServerEnabledChange,
  onMcpServerSettings,
}: AwakenConnectorsProps) {
  return (
    <section className="aw-connectors" aria-label="连接器">
      <div className="aw-connectors__inner">
        <header className="aw-connectors__header">
          <div>
            <h1>连接器</h1>
            <p>管理 MCP servers 与 CLI 连接器，连接外部系统和工具。</p>
          </div>
          <div className="aw-connectors__header-actions">
            <button type="button" className="aw-connectors__refresh" aria-label="刷新" disabled={loading} onClick={onRefresh}>
              <AwakenIcon concept="refresh" size={16} />
            </button>
            {activeKind === "mcp" ? (
              <AwakenButton variant="primary" size="sm" onClick={onConfigureMcp}>
                <AwakenIcon concept="add" size={16} />配置 MCP
              </AwakenButton>
            ) : null}
          </div>
        </header>

        <div className="aw-connectors__toolbar">
          <div className="aw-connectors__tabs" role="tablist" aria-label="连接器分类">
            {(["mcp", "cli"] as const).map((kind) => (
              <button
                key={kind}
                type="button"
                role="tab"
                aria-selected={activeKind === kind}
                data-active={activeKind === kind ? "true" : "false"}
                onClick={() => onKindChange(kind)}
              >
                {kind.toUpperCase()}
              </button>
            ))}
          </div>
          <label className="aw-connectors__search">
            <span>搜索连接器</span>
            <input value={searchValue} placeholder="搜索..." onChange={(event) => onSearchChange(event.target.value)} />
          </label>
        </div>

        {errorMessage ? <div className="aw-connectors__message" role="alert">{errorMessage}</div> : null}

        <div className="aw-connectors__sections">
          {activeKind === "mcp" ? (
            <>
              <section>
                <h2>Awaken 内置</h2>
                <ConnectorGrid
                  items={builtInMcpConnectors}
                  emptyMessage="暂无内置 MCP"
                  onConnectorAction={onConnectorAction}
                  onMcpServerEnabledChange={onMcpServerEnabledChange}
                  onMcpServerSettings={onMcpServerSettings}
                />
              </section>
              <section>
                <h2>用户安装</h2>
                <ConnectorGrid
                  items={userMcpConnectors}
                  emptyMessage="暂无用户安装 MCP"
                  onConnectorAction={onConnectorAction}
                  onMcpServerEnabledChange={onMcpServerEnabledChange}
                  onMcpServerSettings={onMcpServerSettings}
                />
              </section>
            </>
          ) : (
            <section aria-label="CLI">
              <ConnectorGrid
                items={cliConnectors}
                emptyMessage="暂无 CLI"
                onConnectorAction={onConnectorAction}
                onMcpServerEnabledChange={onMcpServerEnabledChange}
                onMcpServerSettings={onMcpServerSettings}
              />
            </section>
          )}
        </div>
      </div>
    </section>
  );
}
