import type { ComponentProps } from "react";
import type { AwakenMarkdownAnswer } from "./AwakenMarkdownAnswer";

export type AwakenMarkdownAnswerState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenMarkdownAnswer>>;
};

export const AwakenMarkdownAnswerStates: AwakenMarkdownAnswerState[] = [
  {
    id: "streaming-answer",
    label: "生成中",
    props: {
      markdown: "我会先确认组件实现，再同步更新文档和状态示例。\n\n- 已读取 props\n- 正在替换占位状态",
      status: "active",
    } as unknown as Partial<ComponentProps<typeof AwakenMarkdownAnswer>>,
  },
  {
    id: "done-answer",
    label: "已完成",
    props: {
      markdown: "已完成本组组件说明。\n\n1. demo 头部改为具体用途句\n2. README 增加 `用途` 边界\n3. states 改为中文语义示例",
      status: "done",
    } as unknown as Partial<ComponentProps<typeof AwakenMarkdownAnswer>>,
  },
  {
    id: "error-answer",
    label: "错误回答",
    props: {
      markdown: "生成回答时遇到错误。\n\n- 需要检查 Markdown 输入\n- 保留当前状态，等待父级重试",
      status: "error",
    } as unknown as Partial<ComponentProps<typeof AwakenMarkdownAnswer>>,
  },
  {
    id: "long-answer",
    label: "长回答",
    props: {
      markdown: "这是一段较长的 Agent 回答，用于验证简化 Markdown 渲染器在检查器宽度内的换行、滚动和行内代码展示。父级传入 `markdown` 和 `status`，组件只负责解析段落、列表以及 `inline code`，不会处理表格、标题层级或代码块高亮。\n\n- 第一条说明父级负责消息来源\n- 第二条说明组件负责展示边界\n- 第三条说明长文本应在容器内保持可读",
      status: "done",
    } as unknown as Partial<ComponentProps<typeof AwakenMarkdownAnswer>>,
  },
];
