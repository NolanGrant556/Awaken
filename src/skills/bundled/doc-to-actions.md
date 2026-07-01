---
name: doc-to-actions
description: 提取文档中的待办事项和任务，转换为结构化的行动清单
arguments:
  - name: doc_text
    type: string
    description: 文档内容
    required: true
context: inline
agent: Planner
---

# 文档转行动

TODO: 此 Skill 文案将在产品侧 follow-up 中打磨。

当前骨架行为：分析 `${doc_text}`，提取其中的所有待办事项和隐含任务。

## 执行步骤

1. 扫描 `${doc_text}` 全文
2. 识别显式待办（checkbox、TODO、action item 标记）
3. 识别隐含任务（建议项、待确认项）
4. 按优先级排序输出任务清单