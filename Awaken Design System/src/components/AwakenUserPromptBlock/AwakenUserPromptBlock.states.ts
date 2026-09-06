import type { ComponentProps } from "react";
import type { AwakenUserPromptBlock } from "./AwakenUserPromptBlock";

export type AwakenUserPromptBlockState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenUserPromptBlock>>;
};

export const AwakenUserPromptBlockStates: AwakenUserPromptBlockState[] = [
  {
    id: "prompt",
    label: "用户提示",
    props: {
      text: "请检查 Awaken Design System 里这一组组件的用途说明是否准确。",
      timestampLabel: "09:53",
      timestampDateTime: "2026-09-06T01:53:00.000Z",
      tokens: [
        {
          id: "skill:design-review:user",
          type: "skill",
          label: "design-review",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
  {
    id: "follow-up",
    label: "追问提示",
    props: {
      text: "继续上一轮修改，再检查 states 里是否还有临时示例文案。",
      timestampLabel: "星期六 09:53",
      timestampDateTime: "2026-09-05T01:53:00.000Z",
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
  {
    id: "mixed-token-typography",
    label: "中英文、数字与多种 Token 混排",
    props: {
      text: "nimen 123",
      tokens: [
        {
          id: "capability:computer-use",
          type: "capability",
          label: "Computer Use",
        },
        {
          id: "skill:code-search:user",
          type: "skill",
          label: "code-search",
          leadingText: "我们 women ",
        },
        {
          id: "plugin:business-review:digest",
          type: "plugin",
          label: "Business Review",
          leadingText: "2026 ",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
  {
    id: "workflow-token",
    label: "深度研究 Workflow Token",
    props: {
      text: "核验这项技术变化。",
      tokens: [
        {
          id: "workflow:deep-research",
          type: "workflow",
          label: "深度研究",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
  {
    id: "long-content",
    label: "长提示",
    props: {
      text:
        "1. 读取当前组件实现\n2. 对照运行叙事结构\n3. 保留中间区域布局\n4. 检查气泡默认高度\n5. 点击后展开更多内容\n6. 滚动条只出现在气泡内部\n7. 不增加额外说明文字\n8. 不改 runtime 逻辑\n9. 继续观察边界\n10. 验证最终视觉效果\n11. 确认不会遮挡输入框\n12. 输出简短结论",
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
];
