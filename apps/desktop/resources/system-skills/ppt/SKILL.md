---
name: ppt
version: 2.2.0
description: 读取、创建或编辑 PowerPoint 演示文稿（PPTX/POTX）时使用。
user-invocable: true
---

# Awaken PPT

阅读演示稿或交付可编辑 PPTX。内容、结构与视觉各有证据，不能互相替代。

| 当前任务 | 按需路线 |
|---|---|
| 阅读／摘要 | markitdown 提取逐页文本；问题依赖版式、图表或图片时用包内 thumbnail/render_slides 补视觉，不为文字摘要生成新 PPTX |
| 从零创建 | [PptxGenJS](references/pptxgenjs.md)；版式需要时读 [设计](references/设计.md) |
| 模板修改、增删页、合并拆分 | [编辑](references/editing.md)：先做布局地图，再局部修改或安全 OOXML 编辑 |
| 新建／编辑后的内容、结构和逐页视觉校验 | [输出 QA](references/qa.md) |
| 执行脚本或依赖恢复 | [依赖与执行](references/依赖与执行.md)，只探测实际 route |

## 共同边界

- helper 支持 Python 3.9+，通过 `${AWAKEN_SKILL_DIR}` 绝对路径执行，复用 `execution_plan.python` 等绝对执行计划。
- JSON helper 核对 Bash outcome 和完整 status/issues；禁止接 `| tail` / `| head` 等吞错管道，失败不得改用裸 LibreOffice/Poppler 绕过 wrapper。
- 保留原件；临时解包、转换、预览使用新路径，不直接删除用户目录文件。
- 文本、图表、形状尽量保留 PowerPoint 原生可编辑对象；保留用户模板，重排前确认任务确实要求改变版式。
- 新建／编辑结果执行内容、结构和逐页视觉 QA。发现问题后修复生成器或模板变换，使用新目录重渲染；首轮无问题时据实记录，不为满足流程虚构一轮修复。
- 只读任务完成回答及覆盖范围说明即可。用户需要的最终 `.pptx` 位于 output root，通过校验后 `artifact_publish`；`.potx` 只作输入模板。
- 对多格式子任务传递对应格式的读写和 QA 边界；结构、视觉通过不证明数字或业务事实正确。
