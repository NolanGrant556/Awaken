---
name: sheet-anomaly-scan
description: 扫描表格数据，检测异常值和数据质量问题
whenToUse: 用户提供了表格数据（CSV/JSON），需要快速扫描异常值、缺失值、重复行时使用
arguments:
  - name: sheet_data
    type: string
    description: 表格数据内容，CSV 或 JSON 格式
    required: true
  - name: columns
    type: string
    description: 要检查的列名，逗号分隔；为空则检查所有数值列
    required: false
    default: ""
  - name: threshold
    type: string
    description: 异常值 Z-score 阈值，默认 3.0
    required: false
    default: "3.0"
context: inline
agent: Reviewer
---

# 表格异常扫描

TODO: 此 Skill 文案将在产品侧 follow-up 中打磨。

当前骨架行为：载入 `${sheet_data}`，对 `${columns}` 中指定的列做 Z-score 异常检测，阈值为 `${threshold}`。

## 执行步骤

1. 解析 `${sheet_data}` 为结构化数据
2. 对指定列计算基本统计量（均值、标准差）
3. 标记 Z-score 超过 `${threshold}` 的异常值
4. 检查缺失值和重复行
5. 输出异常报告