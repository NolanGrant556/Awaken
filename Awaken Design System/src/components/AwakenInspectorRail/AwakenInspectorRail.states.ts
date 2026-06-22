import { createElement, type ComponentProps } from "react";
import type { AwakenInspectorRail } from "./AwakenInspectorRail";

export type AwakenInspectorRailState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenInspectorRail>>;
};

function tabSummary(text: string) {
  return createElement(
    "div",
    {
      style: {
        padding: "8px 10px",
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-control)",
        color: "var(--aw-text-muted)",
        fontSize: "var(--aw-font-size-xs)",
      },
    },
    text,
  );
}

function detail(text: string) {
  return createElement(
    "div",
    {
      style: {
        display: "grid",
        gap: "6px",
        color: "var(--aw-text-primary)",
      },
    },
    createElement("strong", null, "当前详情"),
    createElement("span", null, text),
  );
}

export const AwakenInspectorRailStates: AwakenInspectorRailState[] = [
  {
    id: "changes-panel",
    label: "改动面板",
    props: {
      title: "检查器",
      status: "3 files",
      tabs: tabSummary("改动 3 · 终端 live"),
      children: detail("展示本次执行修改的 README、demo 和 states 文件。"),
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorRail>>,
  },
  {
    id: "no-tabs",
    label: "无标签入口",
    props: {
      title: "运行详情",
      status: "ready",
      tabs: null,
      children: detail("父级没有传入 tabs 时，组件仍然保留标题和可滚动详情区。"),
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorRail>>,
  },
  {
    id: "long-title",
    label: "长标题",
    props: {
      title: "正在查看一条较长的检查器标题，用于验证右侧面板标题截断和 title 提示",
      status: "reviewing",
      tabs: tabSummary("文件 · 终端 · 浏览器"),
      children: detail("详情区内容由父级提供，InspectorRail 只负责容器和滚动。"),
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorRail>>,
  },
];
