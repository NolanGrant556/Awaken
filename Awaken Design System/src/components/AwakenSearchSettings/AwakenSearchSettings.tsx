import "./AwakenSearchSettings.css";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenSwitch } from "../AwakenSwitch";

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
              <AwakenIcon concept="search" size={20} />
            </div>
            <div className="search-channel-copy">
              <div className="search-channel-title-row">
                <h3>{provider.name}</h3>
                <span className={`extension-state ${provider.enabled ? "extension-state-connected" : "extension-state-disabled"}`}>
                  {provider.enabled ? "已开启" : "已关闭"}
                </span>
                <div className="search-channel-meta"><span>内置</span><span>免 key</span></div>
              </div>
              <p>{provider.description}</p>
              {provider.testMessage ? <div className="search-channel-test-result" role="status">{provider.testMessage}</div> : null}
            </div>
            <div className="search-channel-actions">
              <button
                type="button"
                className="search-channel-test-button"
                disabled={provider.testing || provider.disabled || provider.testDisabled}
                onClick={() => onTestProvider?.(provider.id)}
              >
                <AwakenIcon concept="network" size={16} />
                {provider.testing ? "测试中…" : "测试连接"}
              </button>
              <AwakenSwitch
                checked={provider.enabled}
                label={provider.name}
                disabled={provider.disabled || provider.toggleDisabled}
                onCheckedChange={() => onToggleProvider?.(provider.id)}
              />
            </div>
          </article>
        ))}
      </div>
      {message ? <div className="model-settings-message" role="status">{message}</div> : null}
    </section>
  );
}
