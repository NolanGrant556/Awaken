import type { ComponentProps } from "react";
import type { AwakenRunStep } from "./AwakenRunStep";

export type AwakenRunStepState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenRunStep>>;
};

export const AwakenRunStepStates: AwakenRunStepState[] = [
  {
    id: "active",
    label: "进行中",
    props: {
      index: 2,
      title: "读取组件实现",
      description: "根据 props 和渲染逻辑确认组件真实用途。",
      tag: "tsx",
      status: "active",
    } as unknown as Partial<ComponentProps<typeof AwakenRunStep>>,
  },
  {
    id: "pending",
    label: "等待执行",
    props: {
      index: 3,
      title: "更新 README 用途段",
      description: "等待完成 states 文案后统一检查。",
      tag: "docs",
      status: "pending",
    } as unknown as Partial<ComponentProps<typeof AwakenRunStep>>,
  },
  {
    id: "done",
    label: "已完成",
    props: {
      index: 1,
      title: "确认修改范围",
      description: "仅修改第 3 组组件目录内的 demo、states 和 README。",
      tag: "scope",
      status: "done",
    } as unknown as Partial<ComponentProps<typeof AwakenRunStep>>,
  },
  {
    id: "error",
    label: "出错",
    props: {
      index: 4,
      title: "状态数据校验失败",
      description: "发现示例 props 仍缺少真实语义说明，需要重新整理。",
      tag: "states",
      status: "error",
    } as unknown as Partial<ComponentProps<typeof AwakenRunStep>>,
  },
  {
    id: "long-text",
    label: "长文本",
    props: {
      index: 5,
      title: "检查运行时间线中一条非常长的步骤标题是否会正确截断并保留 title 提示",
      description: "用于验证复杂任务说明在紧凑时间线里仍能保持可读。",
      tag: "design-system-docs",
      status: "active",
    } as unknown as Partial<ComponentProps<typeof AwakenRunStep>>,
  },
];
