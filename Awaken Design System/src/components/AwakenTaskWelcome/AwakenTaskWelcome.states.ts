import type { ComponentProps } from "react";
import type { AwakenTaskWelcome } from "./AwakenTaskWelcome";

export type AwakenTaskWelcomeState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenTaskWelcome>>;
};

export const AwakenTaskWelcomeStates: AwakenTaskWelcomeState[] = [
  {
    id: "default",
    label: "默认空任务欢迎区",
    props: {},
  },
  {
    id: "short-message",
    label: "短说明文案",
    props: {
      message: "本地运行，安全可控。",
    },
  },
  {
    id: "long-message",
    label: "较长说明文案",
    props: {
      message: "把复杂任务交给 Awaken，从规划到执行保持清晰、可审查、可继续。",
    },
  },
];
