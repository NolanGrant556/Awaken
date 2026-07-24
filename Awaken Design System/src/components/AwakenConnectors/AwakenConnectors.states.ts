import type {
  AwakenCliConnectorReadiness,
  AwakenConnectorCardItem,
  AwakenMcpLogItem,
  AwakenMcpToolItem,
} from "./AwakenConnectors.types";

export const AwakenBuiltInMcpConnectorStates: AwakenConnectorCardItem[] = [
  {
    id: "chrome-devtools",
    variant: "catalog",
    displayName: "Chrome DevTools MCP",
    description: "Chrome DevTools MCP",
    state: "disconnected",
    stateLabel: "未连接",
    tags: ["开发", "内置"],
    connected: false,
  },
  {
    id: "context7",
    variant: "catalog",
    displayName: "context7",
    description: "context7",
    state: "disconnected",
    stateLabel: "未连接",
    tags: ["数据", "内置"],
    connected: false,
  },
  {
    id: "github",
    variant: "catalog",
    displayName: "GitHub",
    description: "GitHub",
    state: "disconnected",
    stateLabel: "未连接",
    tags: ["开发", "内置"],
    connected: false,
  },
  {
    id: "playwright",
    variant: "catalog",
    displayName: "Playwright",
    description: "Playwright",
    state: "connected",
    stateLabel: "已连接",
    tags: ["开发", "内置", "playwright"],
    connected: true,
    enabled: true,
    mcpServerName: "playwright",
  },
  {
    id: "puppeteer",
    variant: "catalog",
    displayName: "Puppeteer",
    description: "Puppeteer",
    state: "connected",
    stateLabel: "已连接",
    tags: ["开发", "内置", "puppeteer"],
    connected: true,
    enabled: true,
    mcpServerName: "puppeteer",
  },
];

export const AwakenUserMcpConnectorStates: AwakenConnectorCardItem[] = [
  {
    id: "filesystem-user",
    variant: "server",
    displayName: "filesystem",
    description: "stdio · user",
    state: "connected",
    stateLabel: "已连接",
    enabled: true,
  },
  {
    id: "postgres-user",
    variant: "server",
    displayName: "postgres",
    description: "stdio · user",
    state: "connected",
    stateLabel: "已连接",
    enabled: true,
  },
];

export const AwakenCliConnectorStates: AwakenConnectorCardItem[] = [
  {
    id: "dingtalk-workspace",
    variant: "cli",
    displayName: "钉钉",
    description: "连接钉钉，管理日历、联系人、群聊、任务和审批。",
    brandTone: "dingtalk",
    connected: true,
  },
  {
    id: "lark-cli",
    variant: "cli",
    displayName: "飞书",
    description: "连接飞书，通过官方 CLI 授权调用日历、云文档与消息能力。",
    brandTone: "lark",
    connected: true,
  },
];

export const AwakenCliInstalledLoggedOutState: AwakenCliConnectorReadiness = {
  install: "installed",
  executable: "available",
  authentication: "required",
};

export const AwakenCliConnectedState: AwakenCliConnectorReadiness = {
  install: "installed",
  executable: "available",
  authentication: "authorized",
  account: { displayName: "Nolan", organization: "Awaken" },
};

export const AwakenMcpToolStates: AwakenMcpToolItem[] = [
  { name: "browser_navigate", description: "打开指定页面并等待加载完成。", readOnly: false },
  { name: "browser_snapshot", description: "读取当前页面的可访问性快照。", readOnly: true },
];

export const AwakenMcpLogStates: AwakenMcpLogItem[] = [
  { id: 1, timestamp: Date.UTC(2026, 6, 20, 8, 30), stream: "stdout", line: "MCP server connected" },
  { id: 2, timestamp: Date.UTC(2026, 6, 20, 8, 31), stream: "stderr", line: "Reconnect scheduled after transport close" },
];

export const AwakenMcpConfigExample = `{
  "mcpServers": {
    "example-server": {
      "command": "npx",
      "args": ["-y", "mcp-server-example"]
    }
  }
}`;

export const AwakenMcpStreamableHttpConfigExample = `{
  "mcpServers": {
    "remote-example": {
      "type": "streamable-http",
      "url": "https://mcp.example.test/mcp"
    }
  }
}`;
