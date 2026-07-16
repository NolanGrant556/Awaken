import { useEffect, useState } from "react";
import { Search, Wifi } from "lucide-react";
import type {
  SearchProviderId,
  SearchSettings,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";

const PROVIDERS: Array<{
  id: SearchProviderId;
  name: string;
  description: string;
}> = [
  {
    id: "exa",
    name: "Exa",
    description: "面向 Agent 的网页检索，无需配置凭据。",
  },
  {
    id: "tavily",
    name: "Tavily",
    description: "返回适合 LLM 消费的排序摘要，无需配置凭据。",
  },
];

export function SearchSettingsPanel({
  runtimeClient,
}: {
  runtimeClient: RuntimeClient;
}) {
  const [settings, setSettings] = useState<SearchSettings | null>(null);
  const [togglingProvider, setTogglingProvider] =
    useState<SearchProviderId | null>(null);
  const [testingProviders, setTestingProviders] = useState<
    Partial<Record<SearchProviderId, boolean>>
  >({});
  const [message, setMessage] = useState<string | null>(null);
  const [testMessages, setTestMessages] = useState<
    Partial<Record<SearchProviderId, string>>
  >({});

  useEffect(() => {
    let live = true;
    void runtimeClient
      .getSearchSettings()
      .then((next) => {
        if (live) setSettings(next);
      })
      .catch((error: unknown) => {
        if (live) {
          setMessage(
            error instanceof Error ? error.message : "无法读取搜索设置。",
          );
        }
      });
    return () => {
      live = false;
    };
  }, [runtimeClient]);

  const toggleProvider = async (provider: SearchProviderId) => {
    if (!settings) return;
    const enabled = !settings.providers[provider].enabled;
    setTogglingProvider(provider);
    setMessage(null);
    try {
      const result = await runtimeClient.saveSearchSettings({
        provider,
        enabled,
      });
      setSettings(result.settings);
      setMessage(
        result.restartStatus === "restarted"
          ? `${PROVIDERS.find((item) => item.id === provider)?.name} 已${enabled ? "开启" : "关闭"}。`
          : result.message ?? "设置已保存，但 Core 重启失败。",
      );
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "更新搜索渠道失败。",
      );
    } finally {
      setTogglingProvider(null);
    }
  };

  const testProvider = async (provider: SearchProviderId) => {
    setTestingProviders((current) => ({ ...current, [provider]: true }));
    setTestMessages((current) => ({ ...current, [provider]: undefined }));
    try {
      const result = await runtimeClient.testSearchProvider({ provider });
      setTestMessages((current) => ({
        ...current,
        [provider]: result.message,
      }));
    } catch (error) {
      setTestMessages((current) => ({
        ...current,
        [provider]:
          error instanceof Error ? error.message : "连接测试失败。",
      }));
    } finally {
      setTestingProviders((current) => ({ ...current, [provider]: false }));
    }
  };

  return (
    <section className="search-settings" aria-label="搜索">
      <div className="search-settings-intro">
        <h2>搜索渠道</h2>
        <p>同时开启时优先使用 Exa；只有 Exa 请求失败才会改用 Tavily。</p>
      </div>
      <div className="search-channel-list">
        {PROVIDERS.map((provider) => {
          const enabled = settings?.providers[provider.id].enabled ?? false;
          const testing = testingProviders[provider.id] === true;
          return (
            <article className="search-channel-card" key={provider.id}>
              <div className="search-channel-icon" aria-hidden="true">
                <Search size={20} strokeWidth={1.9} />
              </div>
              <div className="search-channel-copy">
                <div className="search-channel-title-row">
                  <h3>{provider.name}</h3>
                  <span
                    className={`extension-state ${
                      enabled
                        ? "extension-state-connected"
                        : "extension-state-disabled"
                    }`}
                  >
                    {enabled ? "已开启" : "已关闭"}
                  </span>
                </div>
                <p>{provider.description}</p>
                <div className="search-channel-meta">
                  <span>内置</span>
                  <span>免 key</span>
                </div>
                {testMessages[provider.id] ? (
                  <div className="search-channel-test-result" role="status">
                    {testMessages[provider.id]}
                  </div>
                ) : null}
              </div>
              <div className="search-channel-actions">
                <button
                  type="button"
                  className="search-channel-test-button"
                  disabled={testing || togglingProvider !== null}
                  onClick={() => void testProvider(provider.id)}
                >
                  <Wifi size={15} strokeWidth={1.9} />
                  {testing ? "测试中…" : "测试连接"}
                </button>
                <button
                  type="button"
                  className={`mcp-toggle-switch${
                    enabled ? " mcp-toggle-switch-on" : " mcp-toggle-switch-off"
                  }`}
                  aria-label={`${enabled ? "关闭" : "开启"} ${provider.name}`}
                  aria-pressed={enabled}
                  disabled={!settings || testing || togglingProvider !== null}
                  onClick={() => void toggleProvider(provider.id)}
                >
                  <span className="mcp-toggle-switch-thumb" />
                </button>
              </div>
            </article>
          );
        })}
      </div>
      {message ? (
        <div className="model-settings-message" role="status">
          {message}
        </div>
      ) : null}
    </section>
  );
}
