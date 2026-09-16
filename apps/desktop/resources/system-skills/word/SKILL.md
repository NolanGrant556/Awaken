---
name: word
version: 2.3.0
description: 读取、创建或编辑 Word 文档（DOCX/DOTX）时使用；复杂模板编辑保留结构与版式。
user-invocable: true
---

# Awaken Word

根据任务读取、创建或编辑 Word 文件。只读任务完成回答并说明覆盖范围；只有用户需要生成或修改文件时才执行输出 QA 和 `artifact_publish`，不为摘要另造文档。

## 共享执行环境

执行 helper 前，通过 `tool_find` 找到 `load_workspace_dependencies`，以 `{}` 取得整套办公环境。每次 Bash 显式传入本 Agent 的 `environment_ref`，使用返回的解释器与启动参数；切换 Python/Node 或项目复用底层安装，各任务取得自己的引用。

缺失或损坏时由同一入口准备或恢复。失败如实报告原因，不改用系统软件、`ensure_environment` 或项目/全局安装补齐办公包。环境、文件内容、结构、视觉与发布分别验收。

## 按需选路

| 当前任务 | 阅读的资源与执行路线 |
|---|---|
| 阅读、摘录、总结 DOCX/DOTX | [读取](references/读取.md)：包内 `scripts/read_docx.py` 输出 Markdown、blocks、warnings、stats |
| 从零创建 | [创建细则](references/creation.md)：Node docx 或适合简单结构的 python-docx |
| 编辑、模板、批注或修订 | [编辑](references/editing.md)：按结构选择局部编辑或安全 OOXML 路线 |
| 执行脚本或处理依赖错误 | [依赖与执行](references/依赖与执行.md)：只探测本次 route，复用绝对执行计划 |
| 生成／编辑后的内容、结构和视觉校验 | [输出 QA](references/qa.md)：每页有实际检查证据，再发布 |

## 共同边界

- `.doc` 先用 LibreOffice 输出新的 `.docx`；`.dotx` 可作模板，正式 Word 交付输出 `.docx`。
- 包内 helper 支持 Python 3.9+；通过 `${AWAKEN_SKILL_DIR}` 的绝对路径调用，后续解释器复用 `execution_plan.python`。
- DOCX 阅读使用标准库 reader；warning 涉及图片、布局、复杂公式或嵌入对象且影响问题时，补视觉路线或明确未覆盖内容。
- 编辑默认输出新文件。复杂页眉页脚、域、目录、文本框、批注、修订和嵌入对象不能默认经 python-docx 整体读写；外部 OOXML 使用包内安全解包器。
- JSON helper 核对 Bash outcome 和完整 JSON；禁止接 `| tail` / `| head` 等吞错管道，失败不得改用裸 LibreOffice/Poppler 绕过 wrapper。
- 多格式任务仅加载实际需要的 Skill；派发子任务时传递相应读写与 QA 边界。
- 用户需要的最终文件位于任务 output root，完成相应校验后 `artifact_publish`；可打开或结构通过都不等于业务内容正确。
