import type { ComponentProps } from "react";
import type { AwakenDisclosure } from "./AwakenDisclosure";

export type AwakenDisclosureState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenDisclosure>>;
};

const noop = () => {};

export const AwakenDisclosureStates: AwakenDisclosureState[] = [
  {
    id: "run-detail-open",
    label: "展开执行详情",
    props: {
      title: "正在检查 Design System 组件说明",
      children: "已读取 .tsx 实现、demo 和 README，准备同步用途说明。",
      meta: "3 条记录",
      defaultOpen: true,
      onOpenChange: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenDisclosure>>,
  },
  {
    id: "terminal-output-collapsed",
    label: "默认收起",
    props: {
      title: "终端输出",
      children: "pnpm check 暂未运行，展开后可查看命令输出摘要。",
      meta: "可选",
      defaultOpen: false,
    } as unknown as Partial<ComponentProps<typeof AwakenDisclosure>>,
  },
  {
    id: "controlled-open",
    label: "受控打开",
    props: {
      title: "工具调用参数",
      children: "父级用 open 控制当前面板保持展开，组件只回传下一次开合状态。",
      meta: "受控",
      open: true,
      onOpenChange: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenDisclosure>>,
  },
  {
    id: "long-title",
    label: "长标题",
    props: {
      title: "正在复核一组较长的组件说明，确保标题在窄检查器中仍然可读并正确换行",
      children: "用于验证标题、meta 和正文在受限宽度里的排布。",
      meta: "12 KB",
      defaultOpen: true,
    } as unknown as Partial<ComponentProps<typeof AwakenDisclosure>>,
  },
];
