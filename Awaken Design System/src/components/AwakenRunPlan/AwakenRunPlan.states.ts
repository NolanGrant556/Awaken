import type { AwakenRunPlanItemView } from "./AwakenRunPlan";

export type AwakenRunPlanState = {
  id: string;
  label: string;
  items: AwakenRunPlanItemView[];
  explanation?: string;
  paused?: boolean;
};

export const AwakenRunPlanStates: AwakenRunPlanState[] = [
  {
    id: "in-progress",
    label: "进行中（含 activeForm）",
    items: [
      {
        step: "梳理运行计划的 Core 快照与校验规则",
        activeForm: "正在梳理运行计划的 Core 快照与校验规则",
        status: "completed",
      },
      {
        step: "实现运行计划协议与 Desktop 投影",
        activeForm: "正在实现运行计划协议",
        status: "in_progress",
      },
      {
        step: "补齐自动化与真实 Desktop 验收",
        activeForm: "正在补齐自动化与真实 Desktop 验收",
        status: "pending",
      },
    ],
  },
  {
    id: "paused",
    label: "用户暂停（保留当前步骤）",
    paused: true,
    items: [
      {
        step: "梳理运行计划的 Core 快照与校验规则",
        activeForm: "正在梳理运行计划的 Core 快照与校验规则",
        status: "completed",
      },
      {
        step: "实现运行计划协议与 Desktop 投影",
        activeForm: "正在实现运行计划协议",
        status: "in_progress",
      },
      {
        step: "补齐自动化与真实 Desktop 验收",
        activeForm: "正在补齐自动化与真实 Desktop 验收",
        status: "pending",
      },
    ],
  },
  {
    id: "first-step",
    label: "首步进行中",
    items: [
      {
        step: "调查现有 ComposerDock 测量链路",
        activeForm: "正在调查现有 ComposerDock 测量链路",
        status: "in_progress",
      },
      {
        step: "接入进度入口与展开面板",
        activeForm: "正在接入进度入口与展开面板",
        status: "pending",
      },
      {
        step: "验证滚动与焦点恢复",
        activeForm: "正在验证滚动与焦点恢复",
        status: "pending",
      },
    ],
  },
  {
    id: "last-step",
    label: "末步进行中",
    items: [
      {
        step: "建立计划快照",
        activeForm: "正在建立计划快照",
        status: "completed",
      },
      {
        step: "接入展开面板",
        activeForm: "正在接入展开面板",
        status: "completed",
      },
      {
        step: "完成真实 Desktop 验收",
        activeForm: "正在完成真实 Desktop 验收",
        status: "in_progress",
      },
    ],
  },
  {
    id: "all-completed",
    label: "全部完成",
    items: [
      {
        step: "建立计划快照",
        activeForm: "正在建立计划快照",
        status: "completed",
      },
      {
        step: "接入展开面板",
        activeForm: "正在接入展开面板",
        status: "completed",
      },
      {
        step: "完成真实 Desktop 验收",
        activeForm: "正在完成真实 Desktop 验收",
        status: "completed",
      },
    ],
  },
  {
    id: "with-explanation",
    label: "计划变更含 explanation",
    explanation: "调查后发现需要额外的协议兼容步骤，已调整计划。",
    items: [
      {
        step: "调查现有协议兼容范围",
        activeForm: "正在调查现有协议兼容范围",
        status: "completed",
      },
      {
        step: "补充协议兼容步骤",
        activeForm: "正在补充协议兼容步骤",
        status: "in_progress",
      },
      {
        step: "回归验证既有 wire 事件",
        activeForm: "正在回归验证既有 wire 事件",
        status: "pending",
      },
    ],
  },
  {
    id: "long-steps-overflow",
    label: "长步骤换行与面板内滚动",
    items: [
      {
        step: "已完成一项带有非常长文案的步骤，用来验证面板内文字会自然换行而不是产生水平滚动条，已完成一项带有非常长文案的步骤",
        activeForm: "正在处理一项带有非常长文案的步骤",
        status: "completed",
      },
      {
        step: "正在进行一项带有非常长文案的步骤，用来验证进行中强调文字在窄窗下仍然换行展示，不会被截断或推动横向滚动",
        activeForm: "正在处理一项带有非常长文案且仍在进行的步骤",
        status: "in_progress",
      },
      {
        step: "待处理步骤三：验证面板 body 独立滚动",
        activeForm: "正在处理待处理步骤三",
        status: "pending",
      },
      {
        step: "待处理步骤四：验证滚动不传递给 RunStage",
        activeForm: "正在处理待处理步骤四",
        status: "pending",
      },
      {
        step: "待处理步骤五：验证窄 thumb 滚动条",
        activeForm: "正在处理待处理步骤五",
        status: "pending",
      },
      {
        step: "待处理步骤六：验证高度上限 360px",
        activeForm: "正在处理待处理步骤六",
        status: "pending",
      },
      {
        step: "待处理步骤七：验证超出后只滚动面板 body",
        activeForm: "正在处理待处理步骤七",
        status: "pending",
      },
      {
        step: "待处理步骤八：验证步骤顺序保持快照顺序",
        activeForm: "正在处理待处理步骤八",
        status: "pending",
      },
    ],
  },
];
