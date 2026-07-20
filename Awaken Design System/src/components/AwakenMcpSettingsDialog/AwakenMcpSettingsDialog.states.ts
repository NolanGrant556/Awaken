import {
  AwakenMcpConfigExample,
  AwakenMcpLogStates,
  AwakenMcpToolStates,
} from "../AwakenConnectors/AwakenConnectors.states";

export const AwakenMcpSettingsDialogState = {
  open: false,
  serverName: "playwright",
  scopeLabel: "用户",
  configValue: AwakenMcpConfigExample,
  tools: AwakenMcpToolStates,
  logs: AwakenMcpLogStates,
};
