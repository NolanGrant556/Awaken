import type { ComponentProps } from "react";
import type { AwakenReconnectStatus } from "./AwakenReconnectStatus";

export type AwakenReconnectStatusState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenReconnectStatus>>;
};

const noop = () => {};

export const AwakenReconnectStatusStates: AwakenReconnectStatusState[] = [
  {
    id: "waiting-for-network",
    label: "等待网络",
    props: {
      title: "正在等待网络",
      detail: "tls handshake eof",
    },
  },
  {
    id: "reconnecting-bounded",
    label: "有界重连",
    props: {
      title: "正在重新连接 2/5",
      details: ["stream eof", "error sending request"],
      defaultOpen: true,
    },
  },
  {
    id: "controlled-open",
    label: "受控展开",
    props: {
      title: "正在重新连接 1/5",
      detail: "connection reset",
      open: true,
      onOpenChange: noop,
    },
  },
  {
    id: "title-only",
    label: "无可展开细节",
    props: {
      title: "正在等待网络",
    },
  },
];
