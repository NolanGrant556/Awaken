import type { AwakenProfileBalancePanelProps } from "./AwakenProfileBalancePanel";

const noop = () => {};
const sharedControls = {
  themePreference: "light" as const,
  onThemePreferenceChange: noop,
  onOpenSettings: noop,
};

export const AwakenProfileBalanceSamples: NonNullable<
  AwakenProfileBalancePanelProps["balances"]
> = [
  {
    profileId: "deepseek-v4-pro",
    providerId: "deepseek",
    modelId: "deepseek-v4-pro",
    displayName: "DeepSeek V4 Pro",
    status: "available",
    detail: {
      kind: "amount",
      amounts: [{ currency: "CNY", total: 86.4, granted: 20, toppedUp: 66.4 }],
    },
  },
  {
    profileId: "kimi",
    providerId: "kimi-coding-plan",
    modelId: "kimi-for-coding",
    displayName: "Kimi",
    status: "available",
    detail: {
      kind: "quota",
      quotas: [{ name: "每周用量", limit: 100, used: 34, remaining: 66 }],
    },
  },
];

export type AwakenProfileBalancePanelState = {
  id: string;
  label: string;
  props: AwakenProfileBalancePanelProps;
};

export const AwakenProfileBalancePanelStates: AwakenProfileBalancePanelState[] = [
  {
    id: "available",
    label: "可用额度",
    props: { ...sharedControls, balances: AwakenProfileBalanceSamples },
  },
  {
    id: "loading",
    label: "查询中",
    props: { ...sharedControls, loading: true },
  },
  {
    id: "empty",
    label: "暂无可查询模型",
    props: { ...sharedControls, balances: [] },
  },
  {
    id: "error",
    label: "查询失败",
    props: { ...sharedControls, error: "暂时无法读取模型额度" },
  },
];
