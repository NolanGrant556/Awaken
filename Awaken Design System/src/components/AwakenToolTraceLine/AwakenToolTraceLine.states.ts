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
      title: "shell",
      detail: "npm test",
      output: "正在执行测试…",
      outputLabel: "终端输出",
      kind: "terminal",
      status: "active",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "done",
    label: "已完成",
    props: {
      title: "apply_patch",
      detail: "App.tsx",
      output: "Done!",
      status: "done",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "error",
    label: "工具失败",
    props: {
      title: "skill__dws",
      detail: "dws",
      output: "授权已过期，请重新登录。",
      kind: "mcp",
      status: "error",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "static",
    label: "仅展示",
    props: {
      title: "artifact_publish",
      detail: "output/report.pdf",
      status: "done",
      onSelect: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "long-content",
    label: "长工具返回",
    props: {
      title: "skill__dws",
      detail: "dws",
      output:
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
      kind: "skill",
      onSelect: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
];
