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
      title: "正在运行 npm test",
      detailCard: {
        title: "Shell 详情",
        copyText: "$ npm test\n正在执行测试…",
        sections: [
          { label: "命令", content: "npm test", format: "code" },
          { label: "输出", content: "正在执行测试…", format: "code" },
        ],
      },
      kind: "terminal",
      status: "active",
      onCopyDetail: noop,
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "done",
    label: "已完成",
    props: {
      title: "编辑 apps/desktop/src/renderer/App.tsx",
      detailCard: {
        title: "文件变更",
        copyText: "apps/desktop/src/renderer/App.tsx\n@@ -1 +1 @@",
        sections: [
          { label: "文件", content: "apps/desktop/src/renderer/App.tsx", format: "text" },
          { label: "变更", content: "@@ -1 +1 @@\n-old\n+new", format: "diff" },
        ],
      },
      status: "done",
      onCopyDetail: noop,
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "error",
    label: "工具失败",
    props: {
      title: "调用 GitHub create_issue",
      detail: "github:create_issue",
      detailCard: {
        title: "GitHub MCP 详情",
        copyText: "授权已过期，请重新登录。",
        sections: [
          { label: "错误与恢复信息", content: "授权已过期，请重新登录。", format: "text", tone: "danger" },
        ],
      },
      kind: "mcp",
      status: "error",
      onCopyDetail: noop,
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "static",
    label: "仅展示",
    props: {
      title: "发布 output/report.pdf",
      detail: "output/report.pdf",
      status: "done",
      onSelect: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "long-content",
    label: "长工具返回",
    props: {
      title: "使用 dws",
      detailCard: {
        title: "Skill 详情",
        copyText:
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
        sections: [{
          label: "结果",
          content: "已加载钉钉全产品 Skill；可用能力包括 aitable、calendar、doc、drive 等。",
          format: "text",
        }],
      },
      status: "done",
      kind: "skill",
      onCopyDetail: noop,
      onSelect: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  // 八个状态各有唯一图形，这里把此前只覆盖 active / done / error 的状态补齐。
  // 真实运行里 denied / cancelled / interrupted / blocked / unsupported 需要相应的
  // 权限拒绝、取消、中断、策略阻断与工具不可用条件才会出现，Gallery 是它们
  // 唯一可稳定观测的入口。
  {
    id: "denied",
    label: "被拒绝",
    props: {
      title: "操作 点击确认支付",
      detail: "点击「确认支付」",
      status: "denied",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "cancelled",
    label: "已取消",
    props: {
      title: "搜索 季度财报对比",
      detail: "季度财报对比",
      status: "cancelled",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "interrupted",
    label: "被中断",
    props: {
      title: "运行 npm run build",
      detail: "npm run build",
      status: "interrupted",
      kind: "terminal",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "blocked",
    label: "被策略阻断",
    props: {
      title: "查看 远程",
      detail: "push origin main",
      status: "blocked",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "unsupported",
    label: "工具不可用",
    props: {
      title: "使用 图像生成",
      detail: "当前模型不支持图像生成",
      status: "unsupported",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
];
