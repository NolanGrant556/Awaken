import { useEffect, useState } from "react";
import { AwakenSearchSettings } from "@awaken/design-system";
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
    <AwakenSearchSettings
      providers={PROVIDERS.map((provider) => ({
        ...provider,
        enabled: settings?.providers[provider.id].enabled ?? false,
        testing: testingProviders[provider.id] === true,
        testDisabled:
          testingProviders[provider.id] === true || togglingProvider !== null,
        toggleDisabled:
          !settings || testingProviders[provider.id] === true || togglingProvider !== null,
        testMessage: testMessages[provider.id],
      }))}
      message={message}
      onTestProvider={(provider) => void testProvider(provider as SearchProviderId)}
      onToggleProvider={(provider) => void toggleProvider(provider as SearchProviderId)}
    />
  );
}
