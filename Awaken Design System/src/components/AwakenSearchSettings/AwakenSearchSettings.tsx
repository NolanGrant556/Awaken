import "./AwakenSearchSettings.css";
import { Search, Wifi } from "lucide-react";

export type AwakenSearchProviderItem = {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  testing?: boolean;
  disabled?: boolean;
  testDisabled?: boolean;
  toggleDisabled?: boolean;
  testMessage?: string;
};

export type AwakenSearchSettingsProps = {
  providers: AwakenSearchProviderItem[];
  message?: string | null;
  onToggleProvider?: (id: string) => void;
  onTestProvider?: (id: string) => void;
};

export function AwakenSearchSettings({
  providers,
  message,
  onToggleProvider,
  onTestProvider,
}: AwakenSearchSettingsProps) {
  return (
    <section className="search-settings" aria-label="搜索">
      <div className="search-settings-intro">
        <h2>搜索渠道</h2>
        <p>同时开启时优先使用 Exa；只有 Exa 请求失败才会改用 Tavily。</p>
      </div>
      <div className="search-channel-list">
        {providers.map((provider) => (
          <article className="search-channel-card" key={provider.id}>
            <div className="search-channel-icon" aria-hidden="true">
              <Search size={20} strokeWidth={1.9} />
            </div>
            <div className="search-channel-copy">
              <div className="search-channel-title-row">
                <h3>{provider.name}</h3>
                <span className={`extension-state ${provider.enabled ? "extension-state-connected" : "extension-state-disabled"}`}>
                  {provider.enabled ? "已开启" : "已关闭"}
                </span>
              </div>
              <p>{provider.description}</p>
              <div className="search-channel-meta"><span>内置</span><span>免 key</span></div>
              {provider.testMessage ? <div className="search-channel-test-result" role="status">{provider.testMessage}</div> : null}
            </div>
            <div className="search-channel-actions">
              <button
                type="button"
                className="search-channel-test-button"
                disabled={provider.testing || provider.disabled || provider.testDisabled}
                onClick={() => onTestProvider?.(provider.id)}
              >
                <Wifi size={15} strokeWidth={1.9} />
                {provider.testing ? "测试中…" : "测试连接"}
              </button>
              <button
                type="button"
                className={`mcp-toggle-switch${provider.enabled ? " mcp-toggle-switch-on" : " mcp-toggle-switch-off"}`}
                aria-label={`${provider.enabled ? "关闭" : "开启"} ${provider.name}`}
                aria-pressed={provider.enabled}
                disabled={provider.disabled || provider.toggleDisabled}
                onClick={() => onToggleProvider?.(provider.id)}
              >
                <span className="mcp-toggle-switch-thumb" />
              </button>
            </div>
          </article>
        ))}
      </div>
      {message ? <div className="model-settings-message" role="status">{message}</div> : null}
    </section>
  );
}
