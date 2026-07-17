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
    description: "连接飞书，通过官方 CLI 授权调用日历、云文档与消息能力。",
  },
};

export function cliConnectorProductCopy(providerId: string): CliConnectorProductCopy | null {
  if (providerId === "dingtalk-workspace" || providerId === "lark-cli") {
    return catalog[providerId];
  }
  return null;
}
