import type { ComponentProps } from "react";
import type { AwakenAssistantMessage } from "./AwakenAssistantMessage";

export type AwakenAssistantMessageState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenAssistantMessage>>;
};

export const AwakenAssistantMessageStates: AwakenAssistantMessageState[] = [
  {
    id: "streaming-answer",
    label: "流式回答",
    props: {
      markdown: "正在分析当前工作区的设计系统状态，并整理需要补充用途说明的组件...",
      status: "active",
    } as unknown as Partial<ComponentProps<typeof AwakenAssistantMessage>>,
  },
  {
    id: "completed-answer",
    label: "完成回答",
    props: {
      markdown: "已检查 `AwakenComposer` 的 props。它负责组合输入框、模式切换、模型选择和发送按钮，提交后的实际运行由父级处理。\n\n- 保留 Plan/Act 模式\n- 保留停止态按钮\n- 不在组件内访问文件系统",
      status: "done",
    } as unknown as Partial<ComponentProps<typeof AwakenAssistantMessage>>,
  },
  {
    id: "table",
    label: "表格与列表",
    props: {
      markdown: "组件 | 责任\n--- | ---\nAwakenArtifactCard | 展示单个文件产物\nAwakenArtifactGrid | 排列多个产物卡片\n\n1. 父级提供数据\n2. 子组件只负责展示",
      status: "done",
    } as unknown as Partial<ComponentProps<typeof AwakenAssistantMessage>>,
  },
  {
    id: "error",
    label: "错误回答",
    props: {
      markdown: "无法解析这段 markdown 产物，需要保留原始输出并提示用户查看日志。",
      status: "error",
    } as unknown as Partial<ComponentProps<typeof AwakenAssistantMessage>>,
  },
  {
    id: "long-answer",
    label: "长段落",
    props: {
      markdown: "这是一段较长的助手说明，用于验证普通段落在回答气泡中保持可读。组件只处理轻量 markdown 渲染、列表和表格排版，不负责消息流排序、头像、时间戳或复制等外围动作。",
      status: "done",
    } as unknown as Partial<ComponentProps<typeof AwakenAssistantMessage>>,
  },
];
