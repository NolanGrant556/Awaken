---
name: PDF_Max
version: 1.2.0
description: 用 Typst 新建、精排 PDF 或维护现有 Typst 项目时使用；已有 PDF 阅读交给 pdf。
user-invocable: true
---

# PDF_Max

生成可复核的专业 PDF。普通 PDF 加工交给 `pdf`，普通可编辑演示稿交给 `ppt`；只有用户明确要求 Typst/PDF 幻灯片时才选择对应幻灯片引擎。

## 按需路线

| 当前需要 | 读取或执行 |
|---|---|
| 解析 Typst、字体检查、依赖恢复或编译 | [环境与编译](references/环境与编译.md)：scripts/typst_runtime.py 与 scripts/generate_pdf.py |
| 根据内容选择模板／包、处理增强决策 | [路线规划](references/路线规划.md)：scripts/plan_document.py 读取 references/routing-catalog.json |
| 准备报告、Markdown 或已有项目 | [文档基底](references/文档基底.md)：scripts/prepare_document.py 与 assets/ 内基底 |
| 具体 Typst 写法 | [语法示例](references/typst-patterns.md) |
| 使用本包报告基底或用户要求重排 | [排版节奏](references/layout-rhythm.md) |
| 依赖来源与版权 | [第三方说明](references/third-party-notices.md) |

只读取当前路线资源；普通生成不通读 65 项路由目录。包内脚本通过 `${AWAKEN_SKILL_DIR}` 绝对路径执行。

## 能力与内容边界

- 本 Skill 不读取已有 PDF。阅读／摘要任务先 `Read`；`model_pdf_unsupported` 时停止并请用户换模型，不安装 Typst、Python、Poppler/OCR 绕过。
- Typst 生成路线可以按需安装 CLI；复用 resolver 返回的绝对 `TYPST_BIN`，不能假定 PATH 热刷新。
- 用户或出版规范优先，其次用户模板、已选包与本 Skill 默认。每个冲突组只保留一个 owner；已有项目保留入口与样式。
- 字体来自真实 `typst fonts`，中文字体通过 `--cjk-font` 指定；不虚构作者、素材或已嵌入字体。
- 只执行规划器的结构化 argv，不把工作区 JSON 当自由 Shell 命令；Markdown 不启用 raw Typst 注入。
- 自动编号不手写重复编号；宽表先调整列宽／字号／横页，图片保留比例，缺素材如实标记并在交付前处理。

## 完成条件

生成后执行一次完整 `--strict` 编译，不能以旧 PDF 或局部编译代替。用 `Read` 检查最终 PDF 的实际页面、表格、图片、公式、字体与分页，发现问题后修复并重新完整编译。

若当前模型返回 `model_pdf_unsupported`，可在编译与 PDF 签名校验通过后发布，明确说明无法视觉复核并建议换模型，不为复核安装 OCR 绕过。其他错误按实际原因恢复，不冒充这一降级。

最终文件位于任务 output root，无示例占位或虚构信息，并通过 `artifact_publish` 交付。报告实际完成的编译、签名、视觉检查与发布结果；新建增强未决定时按路线规划处理，不重复询问已经绑定当前计划的选择。
