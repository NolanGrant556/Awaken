import { createElement } from "react";
import type { ComponentProps } from "react";
import { Settings } from "lucide-react";
import type {
  AwakenComposerBar,
  ComposerBarMetaItem,
} from "./AwakenComposerBar";

export type AwakenComposerBarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenComposerBar>>;
};

const noop = () => {};

const modelMetaItem: ComposerBarMetaItem = {
  id: "model",
  label: "deepseek-v4-pro",
  menuItems: [
    {
      id: "deepseek-v4-pro",
      label: "deepseek-v4-pro",
      icon: "D",
      selected: true,
      onSelect: noop,
    },
    {
      id: "kimi-k2.6",
      label: "kimi-k2.6",
      icon: "K",
      onSelect: noop,
    },
    {
      id: "kimi-for-coding-highspeed",
      label: "kimi-for-coding-highspeed",
      icon: "K",
      onSelect: noop,
    },
  ],
  menuFooterAction: {
    label: "添加模型",
    icon: createElement(Settings, { size: 16, strokeWidth: 1.9 }),
    onClick: noop,
  },
};

const composerMetaItems: ComposerBarMetaItem[] = [
  modelMetaItem,
  { id: "repo", label: "Awaken" },
];

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
      metaItems: composerMetaItems,
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
      metaItems: composerMetaItems,
      projectPicker: {
        placeholder: "选择项目",
        options: [
          { id: "awaken", label: "Awaken", selected: true, onSelect: noop },
          { id: "erp", label: "ERP 项目", onSelect: noop },
          { id: "marketing", label: "市场部", onSelect: noop },
        ],
        createAction: {
          label: "创建项目",
          description: "可选择一个或多个文件夹",
          onClick: noop,
        },
      },
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
