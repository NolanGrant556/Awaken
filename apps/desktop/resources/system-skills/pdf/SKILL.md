---
name: pdf
version: 1.2.0
description: 阅读或加工 PDF 时使用；专业 Typst 排版交给 pdf-max。
user-invocable: true
---

# Awaken PDF

`Read` 负责阅读；Bash 与包内脚本负责用户需要的 PDF 加工。只读任务回答问题并说明页码范围即可；生成／修改文件才执行输出校验与 `artifact_publish`。

## 阅读与能力边界

先 `Read` 目标 PDF。若返回 `model_pdf_unsupported` / `unsupported_media`，立即停止阅读／摘要／看图任务，请用户切换到支持原生 PDF 或视觉的模型。禁止通过 Bash 抽文、pip/uv/venv、Poppler/OCR 或其他 Office Skill 绕过该边界。

加工／生成仅在任务不依赖当前模型看懂源 PDF 时可独立继续；不能冒充已经阅读用户附件。

## 阅读分页策略

页数未知时按以下顺序执行：

1. 首次优先显式传 `pages: "1-10"`。如果模型省略 `pages`，宿主也会把超过 10 页的渲染页路线自动归一化为首段 `1-10`，不会先返回分页参数异常；已知不超过 10 页则仍整份读取，`route=native_pdf` 则表示原生整份已完整交给模型。
2. 从成功结果的 `page_count` 与 `route` 判断后续：`route=native_pdf` 表示整份 PDF 已交给模型，无需重复读取；渲染页路线且 `page_count > 10` 时，继续读取 `11-20`、`21-30` 等剩余范围。
3. 每次最多 20 页；已经知道总页数时，最后一段只请求真实剩余页。

模型应为已知 >10 页的后续读取显式传页码；即使首次遗漏，宿主也不会整份渲染长 PDF。该策略保留上下文保护，同时避免把正常的页数发现过程显示成一次工具异常。

## 按需加工路线

- 合并、拆分、旋转、抽表格或新建：读 [加工与依赖](references/加工与依赖.md)，只准备实际需要的库；复杂水印、加密、已有 OCR 见 [参考](references/reference.md)。
- 填表：读 [表单](references/forms.md)，使用 `scripts/extract_form_field_info.py` 和 `scripts/fill_fillable_fields.py`；共同字段定位由 `scripts/form_fields.py` 拥有。
- 专业报告、论文、简历和精排 Markdown：调用 `PDF_Max`（ID `pdf-max`）；普通可编辑演示稿交给 `ppt`。

## 文件交付

- 包内脚本通过 `${AWAKEN_SKILL_DIR}` 的绝对路径执行；编辑默认新文件，不覆盖源 PDF。
- 生成结果按任务检查实际修改页、字段值和中文字体；视觉能力不足时限定验证结论，不能伪称通过。
- 新文件必须位于任务 output root；完成相应校验后 `artifact_publish`。
