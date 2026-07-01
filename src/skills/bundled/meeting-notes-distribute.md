---
name: meeting-notes-distribute
description: 提取会议纪要中的行动项，分发到对应负责人
whenToUse: 用户提供会议纪要文本，需要从中提取任务并分发给团队成员时使用
arguments:
  - name: meeting_notes
    type: string
    description: 会议纪要的原始文本
    required: true
  - name: participants
    type: string
    description: 参会人列表，格式：姓名(标识符)，逗号分隔
    required: false
    default: ""
context: inline
agent: Operator
---

# 会议纪要分发

TODO: 此 Skill 文案将在产品侧 follow-up 中打磨。

当前骨架行为：解析 `${meeting_notes}` 中的行动项，按 `${participants}` 匹配负责人。

## 执行步骤

1. 解析会议纪要文本，识别所有行动项（action items）
2. 每个行动项提取：任务描述、负责人、截止日期
3. 如果提供了 `${participants}`，按姓名匹配负责人
4. 格式化输出任务清单