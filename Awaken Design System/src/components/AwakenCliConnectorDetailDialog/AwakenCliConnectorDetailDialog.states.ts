import type { AwakenCliConnectorDetailDialogProps } from "./AwakenCliConnectorDetailDialog";

export const AwakenCliConnectorDetailDialogState: Omit<AwakenCliConnectorDetailDialogProps, "onOpenChange" | "onAction"> = {
  open: false,
  providerId: "dingtalk-workspace",
  displayName: "钉钉",
  description: "连接钉钉，管理日历、联系人、群聊、任务和审批。",
  brandTone: "dingtalk",
  readiness: {
    install: "installed",
    executable: "available",
    authentication: "required",
  },
};
