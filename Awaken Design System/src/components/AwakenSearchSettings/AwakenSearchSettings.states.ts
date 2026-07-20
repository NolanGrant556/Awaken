import type { AwakenSearchSettingsProps } from "./AwakenSearchSettings";

export const AwakenSearchSettingsSample: AwakenSearchSettingsProps = {
  providers: [
    { id: "exa", name: "Exa", description: "面向 Agent 的网页检索，无需配置凭据。", enabled: true },
    { id: "tavily", name: "Tavily", description: "返回适合 LLM 消费的排序摘要，无需配置凭据。", enabled: false },
  ],
};
