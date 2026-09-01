import "./AwakenPlugins.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenIcon } from "../AwakenIcon";
import {
  AwakenPluginCard,
  type AwakenPluginCardItem,
} from "../AwakenPluginCard";

export type AwakenPluginsProps = {
  activeView: AwakenPluginsView;
  searchValue: string;
  plugins: AwakenPluginCardItem[];
  loading?: boolean;
  errorMessage?: string | null;
  onSearchChange: (value: string) => void;
  onViewChange: (view: AwakenPluginsView) => void;
  onRefresh: () => void;
  onInstall: () => void;
  onPluginOpen: (plugin: AwakenPluginCardItem) => void;
  onPluginEnabledChange: (
    plugin: AwakenPluginCardItem,
    enabled: boolean,
  ) => void;
};

export type AwakenPluginsView = "builtin" | "installed";

const VIEW_OPTIONS: Array<{ id: AwakenPluginsView; label: string }> = [
  { id: "builtin", label: "内置插件" },
  { id: "installed", label: "已安装插件" },
];

function normalizedIncludes(value: string | undefined, query: string): boolean {
  return Boolean(value?.toLocaleLowerCase().includes(query));
}

export function AwakenPlugins({
  activeView,
  searchValue,
  plugins,
  loading = false,
  errorMessage,
  onSearchChange,
  onViewChange,
  onRefresh,
  onInstall,
  onPluginOpen,
  onPluginEnabledChange,
}: AwakenPluginsProps) {
  const query = searchValue.trim().toLocaleLowerCase();
  const scopedPlugins = plugins
    .filter((plugin) => (activeView === "builtin" ? plugin.builtin === true : plugin.builtin !== true))
    .toSorted((left, right) => {
      const order = (left.builtinOrder ?? 0) - (right.builtinOrder ?? 0);
      if (order !== 0) return order;
      return (left.displayName || left.name).localeCompare(right.displayName || right.name, "zh-CN");
    });
  const visiblePlugins = query
    ? scopedPlugins.filter(
        (plugin) =>
          normalizedIncludes(plugin.name, query) ||
          normalizedIncludes(plugin.displayName, query) ||
          normalizedIncludes(plugin.description, query) ||
          normalizedIncludes(plugin.dialect, query) ||
          normalizedIncludes(plugin.diagnostic, query),
      )
    : scopedPlugins;
  const hasSearchResultGap = query !== "" && scopedPlugins.length > 0;

  return (
    <section className="aw-plugins" aria-label="插件">
      <div className="aw-plugins__inner">
        <header className="aw-plugins__header">
          <div className="aw-plugins__heading">
            <h1>插件</h1>
            <p>
              管理 Awaken 内置能力与已安装插件中的真实组件。
            </p>
          </div>
          <div className="aw-plugins__header-actions">
            <button
              type="button"
              className="aw-plugins__refresh"
              aria-label="刷新"
              disabled={loading}
              onClick={onRefresh}
            >
              <AwakenIcon concept="refresh" size={16} />
            </button>
            <AwakenButton variant="primary" size="lg" onClick={onInstall}>
              <AwakenIcon concept="import" size={16} />
              安装插件
            </AwakenButton>
          </div>
        </header>

        <div className="aw-plugins__toolbar">
          <div className="aw-plugins__tabs" role="tablist" aria-label="插件视图">
            {VIEW_OPTIONS.map((view) => (
              <button
                key={view.id}
                type="button"
                role="tab"
                aria-selected={activeView === view.id}
                className="aw-plugins__tab"
                data-active={activeView === view.id ? "true" : "false"}
                onClick={() => onViewChange(view.id)}
              >
                {view.label}
              </button>
            ))}
          </div>
          <label className="aw-plugins__search">
            <AwakenIcon concept="search" size={16} />
            <span className="aw-plugins__sr-only">搜索插件</span>
            <input
              value={searchValue}
              placeholder="搜索插件…"
              onChange={(event) => onSearchChange(event.target.value)}
            />
          </label>
        </div>

        {errorMessage ? (
          <div className="aw-plugins__message" role="alert">
            {errorMessage}
          </div>
        ) : null}

        <div className="aw-plugins__content" aria-live="polite">
          {visiblePlugins.length > 0 ? (
            <div className="aw-plugins__grid">
              {visiblePlugins.map((plugin) => (
                <AwakenPluginCard
                  key={plugin.name}
                  plugin={plugin}
                  onOpen={() => onPluginOpen(plugin)}
                  onEnabledChange={(enabled) =>
                    onPluginEnabledChange(plugin, enabled)
                  }
                />
              ))}
            </div>
          ) : (
            <div
              className="aw-plugins__empty"
              data-kind={loading ? "loading" : hasSearchResultGap ? "search" : "initial"}
            >
              <span className="aw-plugins__empty-icon" aria-hidden="true">
                <AwakenIcon
                  concept={loading ? "refresh" : hasSearchResultGap ? "search" : "plugin"}
                  size={24}
                />
              </span>
              <div className="aw-plugins__empty-copy">
                <h2>
                  {loading
                    ? "正在加载插件"
                    : hasSearchResultGap
                      ? "没有匹配的插件"
                      : activeView === "builtin"
                        ? "暂无内置插件"
                        : "还没有安装插件"}
                </h2>
                <p>
                  {loading
                    ? "正在读取已安装的插件列表…"
                    : hasSearchResultGap
                      ? "尝试使用插件名称、版本或描述中的其他关键词。"
                      : activeView === "builtin"
                        ? "Awaken 当前没有可用的内置插件。"
                        : "从本地 ZIP 包安装你信任的插件。"}
                </p>
              </div>
              {!loading && !hasSearchResultGap && activeView === "installed" ? (
                <AwakenButton variant="default" size="md" onClick={onInstall}>
                  <AwakenIcon concept="import" size={16} />
                  安装第一个插件
                </AwakenButton>
              ) : null}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
