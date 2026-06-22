import type { ComponentProps } from "react";
import type { AwakenProfileButton } from "./AwakenProfileButton";

export type AwakenProfileButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenProfileButton>>;
};

const noop = () => {};

export const AwakenProfileButtonStates: AwakenProfileButtonState[] = [
  {
    id: "local-profile",
    label: "本机账号",
    props: {
      name: "Nolan Grant",
      subtitle: "本机运行时已连接",
      avatarFallback: "NG",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProfileButton>>,
  },
  {
    id: "without-subtitle",
    label: "仅显示姓名",
    props: {
      name: "本地访客",
      subtitle: undefined,
      avatarFallback: "访",
      avatarSrc: undefined,
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProfileButton>>,
  },
  {
    id: "long-identity",
    label: "长身份信息",
    props: {
      name: "Nolan Grant 的本机 Awaken 开发环境",
      subtitle: "连接到 /Users/nolangrant/Awaken 工作区",
      avatarFallback: "AW",
      avatarSrc: undefined,
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProfileButton>>,
  },
];
