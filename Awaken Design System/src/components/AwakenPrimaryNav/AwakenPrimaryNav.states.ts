import type { ComponentProps } from "react";
import type { AwakenPrimaryNav } from "./AwakenPrimaryNav";

export type AwakenPrimaryNavState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenPrimaryNav>>;
};

const noop = () => {};

export const AwakenPrimaryNavStates: AwakenPrimaryNavState[] = [
  {
    id: "run-navigation",
    label: "运行导航",
    props: {
      items: [
        { id: "runs", label: "运行", badge: 3 },
        { id: "projects", label: "项目" },
        { id: "settings", label: "设置" },
      ],
      activeId: "runs",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenPrimaryNav>>,
  },
  {
    id: "disabled-entry",
    label: "含禁用入口",
    props: {
      items: [
        { id: "runs", label: "运行" },
        { id: "integrations", label: "集成", disabled: true },
        { id: "settings", label: "设置" },
      ],
      activeId: "settings",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenPrimaryNav>>,
  },
  {
    id: "badged-review",
    label: "带计数徽章",
    props: {
      items: [
        { id: "inbox", label: "待处理", badge: 12 },
        { id: "reviews", label: "审查记录", badge: "new" },
        { id: "archive", label: "归档" },
      ],
      activeId: "reviews",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenPrimaryNav>>,
  },
  {
    id: "long-label",
    label: "长导航项",
    props: {
      items: [
        { id: "context", label: "当前工作区上下文与权限记录", badge: 5 },
        { id: "runtime", label: "本机运行时状态" },
      ],
      activeId: "context",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenPrimaryNav>>,
  },
];
