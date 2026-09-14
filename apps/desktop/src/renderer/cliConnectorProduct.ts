import type { CliProviderId } from "../shared/runtime";

export type CliConnectorProductCopy = {
  displayName: string;
  description: string;
};

const catalog: Record<CliProviderId, CliConnectorProductCopy> = {
  "dingtalk-workspace": {
    displayName: "钉钉",
    description: "连接钉钉，管理日历、联系人、群聊、任务和审批。",
  },
  "lark-cli": {
    displayName: "飞书",
    description: "连接飞书，管理日历、云文档与消息。",
  },
};

export function cliConnectorProductCopy(providerId: string): CliConnectorProductCopy | null {
  if (providerId === "dingtalk-workspace" || providerId === "lark-cli") {
    return catalog[providerId];
  }
  return null;
}

/** Product wording only. State and diagnostics codes remain owned by Main. */
export function cliConnectorUserMessage(message: string | undefined): string | undefined {
  return message?.replace(/\bCLI\b/g, "运行组件").replace(/\bSkills?\b/g, "技能");
}

export const builtinConnectorProviders: readonly CliProviderId[] = ["dingtalk-workspace", "lark-cli"];

export function builtinConnectorCardId(providerId: CliProviderId): string {
  return `builtin-connector:${providerId}`;
}

export function builtinConnectorProvider(cardId: string): CliProviderId | undefined {
  return builtinConnectorProviders.find((providerId) => builtinConnectorCardId(providerId) === cardId);
}
