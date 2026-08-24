import "./AwakenPlugins.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenIcon } from "../AwakenIcon";
import {
  AwakenPluginCard,
  type AwakenPluginCardItem,
} from "../AwakenPluginCard";

export type AwakenPluginsProps = {
  searchValue: string;
  plugins: AwakenPluginCardItem[];
  loading?: boolean;
  errorMessage?: string | null;
  onSearchChange: (value: string) => void;
  onRefresh: () => void;
  onInstall: () => void;
  onPluginOpen: (plugin: AwakenPluginCardItem) => void;
  onPluginEnabledChange: (
    plugin: AwakenPluginCardItem,
    enabled: boolean,
  ) => void;
};

function normalizedIncludes(value: string | undefined, query: string): boolean {
  return Boolean(value?.toLocaleLowerCase().includes(query));
}

export function AwakenPlugins({
  searchValue,
  plugins,
  loading = false,
  errorMessage,
  onSearchChange,
  onRefresh,
  onInstall,
  onPluginOpen,
  onPluginEnabledChange,
}: AwakenPluginsProps) {
  const query = searchValue.trim().toLocaleLowerCase();
  const visiblePlugins = query
    ? plugins.filter(
        (plugin) =>
          normalizedIncludes(plugin.name, query) ||
          normalizedIncludes(plugin.version, query) ||
          normalizedIncludes(plugin.description, query) ||
          normalizedIncludes(plugin.diagnostic, query),
      )
    : plugins;
  const hasSearchResultGap = query !== "" && plugins.length > 0;

  return (
    <section className="aw-plugins" aria-label="插件">
      <div className="aw-plugins__inner">
        <header className="aw-plugins__header">
          <div className="aw-plugins__heading">
            <h1>插件</h1>
            <p>
              安装符合 Agent Plugins 1.0.0 的第三方插件包，统一管理其中的 Skills 与 MCP servers。
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
                      : "还没有安装插件"}
                </h2>
                <p>
                  {loading
                    ? "正在读取已安装的插件列表…"
                    : hasSearchResultGap
                      ? "尝试使用插件名称、版本或描述中的其他关键词。"
                      : "从本地目录、zip 文件或 HTTPS URL 安装你信任的插件。"}
                </p>
              </div>
              {!loading && !hasSearchResultGap ? (
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
