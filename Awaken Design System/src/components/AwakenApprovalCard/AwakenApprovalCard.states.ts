import type { ComponentProps } from "react";
import type { AwakenApprovalCard } from "./AwakenApprovalCard";

export type AwakenApprovalCardState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenApprovalCard>>;
};

const noop = () => {};

export const AwakenApprovalCardStates: AwakenApprovalCardState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenApprovalCard",
      description: "AwakenApprovalCard",
      affectedPaths: ["src/Awaken.tsx"],
      patchSummary: "AwakenApprovalCard",
      riskTone: "safe",
      approveLabel: "AwakenApprovalCard",
      rejectLabel: "AwakenApprovalCard",
      resolvedDecision: "AwakenApprovalCard",
      resolving: "AwakenApprovalCard",
      onApprove: noop,
      onReject: noop,
      open: true,
      inline: true,
      onOpenChange: noop,
      children: "AwakenApprovalCard",
      onClick: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenApprovalCard>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenApprovalCard with a deliberately long value that should remain readable inside its container",
      description: "AwakenApprovalCard with a deliberately long value that should remain readable inside its container",
      children: "AwakenApprovalCard with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenApprovalCard>>,
  },
];
