---
name: excel
version: 2.3.0
description: 读取、清洗、创建或编辑本地电子表格（XLSX/XLSM/XLTX/CSV/TSV）时使用。
user-invocable: true
---

# Awaken Excel

按任务分析数据或交付可继续使用的表格。读取、编辑和生成有不同完成条件。

| 当前任务 | 按需资源 |
|---|---|
| 阅读、摘要、跨表分析 | [读取与分析](references/读取与分析.md) |
| 编辑、清洗或创建 | [编辑与保存](references/editing.md) |
| 写入／修改公式、重算及语义校验 | [公式](references/formulas.md) |
| 财务模型且无用户模板约定 | [财务模型](references/financial-model.md) |
| 执行脚本或恢复依赖 | [依赖与执行](references/依赖与执行.md) |

## 共享执行环境

执行 helper 前，通过 `tool_find` 找到 `load_workspace_dependencies`，以 `{}` 取得整套办公环境。每次 Bash 显式传入本 Agent 的 `environment_ref`，使用返回的解释器与启动参数；切换 Python/Node 或项目复用底层安装，各任务取得自己的引用。

缺失或损坏时由同一入口准备或恢复。失败如实报告原因，不改用系统软件、`ensure_environment` 或项目/全局安装补齐办公包。环境、文件内容、结构、视觉与发布分别验收。

## 共同边界

- helper 支持 Python 3.9+，通过 `${AWAKEN_SKILL_DIR}` 绝对路径执行并复用 `execution_plan.python`；只探测当前 route。
- JSON helper 核对完整结果与 Bash outcome；禁止接 `| tail` / `| head` 等吞错管道，失败不得改用裸 soffice 绕过独立 profile、超时与原子替换。
- 用户的 sheet、字段、顺序、格式和公式优先；编辑默认输出新文件，保留原件。不要给既有表擅加样例行或财务配色。
- `data_only=True` workbook 只读缓存，不能保存；宏、外链、动态公式、图表等按编辑路线复核，不能静默降级。
- 只读任务回答问题并说明覆盖范围，不重算或发布原件。创建／修改的 `.xlsx` 含公式时才执行 inspect → recalc → inspect 与关键公式语义抽查。
- `recalc.py` 只接受 `.xlsx`；`.xlsm/.xltx` 可受控分析或编辑，但不通过普通重算路线转换后冒充能力保留。正式交付按用户要求与发布器支持类型输出 `.xlsx/.xls/.csv/.tsv`。
- 生成／编辑完成后检查字段、值、公式、格式和中文显示，最终文件放任务 output root 并 `artifact_publish`。零公式错误不等于业务逻辑正确。
