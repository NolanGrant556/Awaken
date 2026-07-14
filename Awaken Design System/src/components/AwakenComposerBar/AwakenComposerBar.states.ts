import type { ComponentProps } from "react";
import type { AwakenComposerBar } from "./AwakenComposerBar";

export type AwakenComposerBarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenComposerBar>>;
};

const noop = () => {};

export const AwakenComposerBarStates: AwakenComposerBarState[] = [
  {
    id: "dock-empty",
    label: "底部停靠输入",
    props: {
      value: "",
      onChange: noop,
      onSubmit: noop,
      onAddContext: noop,
      placeholder: "给当前运行追加任务...",
      disabled: false,
      isReady: false,
      submitMode: "send",
      variant: "dock",
      metaItems: [
        { id: "repo", label: "Awaken" },
        { id: "model", label: "GPT-5" },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenComposerBar>>,
  },
  {
    id: "center-idle",
    label: "桌面首屏空闲输入",
    props: {
      value: "",
      placeholder: "",
      isReady: false,
      variant: "center",
      metaItems: [],
    } as unknown as Partial<ComponentProps<typeof AwakenComposerBar>>,
  },
  {
    id: "running",
    label: "运行中可停止",
    props: {
      value: "正在检查组件文档。",
      isReady: true,
      submitMode: "stop",
      variant: "dock",
      metaItems: [{ id: "status", label: "运行中" }],
    } as unknown as Partial<ComponentProps<typeof AwakenComposerBar>>,
  },
  {
    id: "disabled",
    label: "输入禁用",
    props: {
      value: "等待运行时恢复连接。",
      disabled: true,
      isReady: false,
      metaItems: [{ id: "status", label: "离线" }],
    } as unknown as Partial<ComponentProps<typeof AwakenComposerBar>>,
  },
  {
    id: "long-draft",
    label: "长任务草稿",
    props: {
      value: "请把 README、demo header 和 states 都改成真实用途描述，特别注意父级负责数据和事件，子级只负责展示与回调。",
      isReady: true,
    } as unknown as Partial<ComponentProps<typeof AwakenComposerBar>>,
  },
  {
    id: "inline-skill-token",
    label: "长前缀中的技能",
    props: {
      value: "这个技能写",
      variant: "center",
      isReady: true,
      tokens: [
        {
          id: "skill:docx:user",
          type: "skill",
          label: "docx",
          leadingText:
            "不行，报告过于简单了，缺乏对资料完整性，你可以直接把资料的内容写进 word 里面的。另外不需要说明基于什么文档而得来的什么说明。而且排版不好看，你使用 ",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenComposerBar>>,
  },
];
