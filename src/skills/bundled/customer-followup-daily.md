---
name: customer-followup-daily
description: 遍历客户列表，生成每日跟进提醒和摘要
whenToUse: 需要批量检查客户跟进状态、生成今日跟进任务清单时使用
arguments:
  - name: customer_list
    type: string
    description: 客户列表，JSON 数组格式
    required: true
  - name: last_contact_window
    type: string
    description: 最后联系窗口天数，超过此天数视为需要跟进
    required: false
    default: "7"
context: fork
agent: Operator
---

# 客户跟进日报

TODO: 此 Skill 文案将在产品侧 follow-up 中打磨。

当前骨架行为：遍历 `${customer_list}`，检查每个客户距离上次联系是否超过 `${last_contact_window}` 天。

## 执行步骤

1. 解析 `${customer_list}` 中的客户数据
2. 对每个客户，计算距离上次联系的天数
3. 超过 `${last_contact_window}` 天的，生成跟进提醒
4. 汇总输出今日跟进优先级清单