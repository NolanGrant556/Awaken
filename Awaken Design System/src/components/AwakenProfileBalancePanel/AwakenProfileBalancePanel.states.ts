import type { AwakenProfileBalancePanelProps } from "./AwakenProfileBalancePanel";

const noop = () => {};
const sharedControls = { onOpenSettings: noop };

export const AwakenProfileBalanceSamples: NonNullable<
  AwakenProfileBalancePanelProps["balances"]
> = [
  {
    profileId: "deepseek-v4-pro",
    providerId: "deepseek",
    providerBrand: "deepseek",
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
    providerBrand: "kimi",
    modelId: "kimi-for-coding",
    displayName: "Kimi",
    status: "available",
    detail: {
      kind: "quota",
      quotas: [{ name: "每周用量", limit: 100, used: 34, remaining: 66 }],
    },
  },
  {
    profileId: "volcengine-agent-plan",
    providerId: "volcengine-agent-plan",
    providerBrand: "volcengine",
    modelId: "ark-code-latest",
    displayName: "火山 Agent Plan",
    status: "requires_setup",
    error: "火山 Agent Plan 推理 Key 不能查询套餐用量；GetAFPUsage 需要独立的火山云 AK/SK 与只读方舟管理权限。",
    setupURL: "https://api.volcengine.com/api-docs/view?action=GetAFPUsage&serviceCode=ark&version=2024-01-01",
    setupLabel: "查看 Agent Plan 用量授权",
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
