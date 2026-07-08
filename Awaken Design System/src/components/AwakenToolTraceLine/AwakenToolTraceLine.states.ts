import type { ComponentProps } from "react";
import type { AwakenToolTraceLine } from "./AwakenToolTraceLine";

export type AwakenToolTraceLineState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenToolTraceLine>>;
};

const noop = () => {};

export const AwakenToolTraceLineStates: AwakenToolTraceLineState[] = [
  {
    id: "active",
    label: "工具执行中",
    props: {
      title: "运行 npm run typecheck",
      detail: "等待 TypeScript 输出。",
      status: "active",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "done",
    label: "已完成",
    props: {
      title: "读取组件实现",
      detail: "AwakenToolTraceLine.tsx 已确认。",
      status: "done",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "error",
    label: "工具失败",
    props: {
      title: "打开 Gallery 预览失败",
      detail: "端口不可用，需要父级展示终端详情。",
      status: "error",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "static",
    label: "仅展示",
    props: {
      title: "记录补充信息",
      detail: "没有选择回调时，这一行不会表现为按钮。",
      status: "done",
      onSelect: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "long-content",
    label: "长工具返回",
    props: {
      title: "skill__dws",
      detail:
        `[skill "dws" executed inline]\n\n` +
        `Base directory for this skill: /Users/nolangrant/.awaken/skills/dws\n` +
        `Use skill_resource_read with skill_id "dws" and a relative path to read bundled references, scripts, or assets from this directory.\n\n` +
        `# 钉钉全产品 Skill\n\n` +
        `通过 dws 命令管理钉钉产品能力。\n\n` +
        `## 严格禁止 (NEVER DO)\n` +
        `- 不要使用 dws 命令以外的方式操作\n` +
        `- 不要编造 UUID、ID 等标识符，必须从命令返回中提取\n` +
        `- 不要猜测字段名/参数值，操作前必须先查询确认\n\n` +
        `## 产品总览\n` +
        `aitable、attendance、calendar、chat、contact、devdoc、ding、doc、drive、minutes、oa、report、mail、sheet、todo、wiki。`,
      status: "done",
      onSelect: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
];
