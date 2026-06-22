import type { ComponentProps } from "react";
import type { AwakenSendButton } from "./AwakenSendButton";

export type AwakenSendButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenSendButton>>;
};

const noop = () => {};

export const AwakenSendButtonStates: AwakenSendButtonState[] = [
  {
    id: "ready",
    label: "可发送",
    props: {
      ready: true,
      onClick: noop,
      disabled: false,
      loading: false,
      mode: "send",
    } as unknown as Partial<ComponentProps<typeof AwakenSendButton>>,
  },
  {
    id: "not-ready",
    label: "输入为空",
    props: {
      ready: false,
      onClick: noop,
      disabled: false,
      loading: false,
      mode: "send",
    } as unknown as Partial<ComponentProps<typeof AwakenSendButton>>,
  },
  {
    id: "loading",
    label: "发送中",
    props: {
      ready: true,
      onClick: noop,
      disabled: false,
      loading: true,
      mode: "send",
    } as unknown as Partial<ComponentProps<typeof AwakenSendButton>>,
  },
  {
    id: "stop",
    label: "停止运行",
    props: {
      ready: true,
      onClick: noop,
      disabled: false,
      loading: false,
      mode: "stop",
    } as unknown as Partial<ComponentProps<typeof AwakenSendButton>>,
  },
  {
    id: "disabled",
    label: "被禁用",
    props: {
      ready: true,
      onClick: noop,
      disabled: true,
      loading: false,
      mode: "send",
    } as unknown as Partial<ComponentProps<typeof AwakenSendButton>>,
  },
];
