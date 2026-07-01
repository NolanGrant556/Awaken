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
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
  {
    id: "follow-up",
    label: "追问提示",
    props: {
      text: "继续上一轮修改，再检查 states 里是否还有临时示例文案。",
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
