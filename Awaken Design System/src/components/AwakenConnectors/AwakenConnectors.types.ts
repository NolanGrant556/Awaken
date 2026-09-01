export type AwakenConnectorKind = "mcp" | "cli";

export type AwakenConnectorState =
  | "disconnected"
  | "pending"
  | "connected"
  | "needs-auth"
  | "approval-required"
  | "failed"
  | "loaded"
  | "invalid"
  | "shadowed"
  | "disabled_by_policy"
  | "disabled";

export type AwakenConnectorCardVariant = "catalog" | "server" | "cli";

export type AwakenConnectorCardItem = {
  id: string;
  variant: AwakenConnectorCardVariant;
  displayName: string;
  description: string;
  state?: AwakenConnectorState;
  stateLabel?: string;
  tags?: string[];
  connected?: boolean;
  authorizing?: boolean;
  showAdminApprovalAction?: boolean;
  enabled?: boolean;
  mcpServerName?: string;
  busy?: boolean;
  brandLogoSrc?: string;
  brandTone?: "dingtalk" | "lark" | "neutral";
  errors?: string[];
};

export type AwakenCliConnectorAction =
  | "install"
  | "cancel_install"
  | "authorize"
  | "cancel_authorization"
  | "retry_authorization"
  | "logout"
  | "unsupported";

export type AwakenCliConnectorReadiness = {
  install: "missing" | "installing" | "installed" | "unsupported" | "failed";
  executable: "unknown" | "missing" | "available" | "invalid";
  authentication: "unknown" | "required" | "in_progress" | "authorized" | "failed";
  account?: {
    displayName?: string;
    organization?: string;
  };
  safeErrorMessage?: string;
};

export type AwakenCliAuthSession = {
  phase: "starting" | "waiting_user" | "confirming" | "authorized" | "failed" | "cancelled";
};

export type AwakenCliInstallProgress = {
  percent: number;
  label: string;
};

export type AwakenMcpToolItem = {
  name: string;
  description: string;
  readOnly: boolean;
};

export type AwakenMcpLogItem = {
  id: string | number;
  timestamp: number;
  stream: "stdout" | "stderr";
  line: string;
};
