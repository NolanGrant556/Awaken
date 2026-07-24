import { AwakenMcpConfigExample } from "../AwakenConnectors/AwakenConnectors.states";

export const AwakenMcpConfigDialogState = {
  open: false,
  mode: "stdio" as const,
  value: AwakenMcpConfigExample,
};
