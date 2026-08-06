---
name: pdf
version: 1.1.2
description: 处理通用 PDF：在模型具备原生 PDF 或视觉能力时阅读/抽取，以及合并拆分、旋转、水印、新建、填表、加密解密、抽图；扫描件 OCR 仅在本机已有工具时。专业长文档、Markdown 精排、论文/简历或明确要求 Typst/PDF 幻灯片时改用 PDF_Max（调用 ID：pdf-max）。
when_to_use: 需要读写、加工或生成通用 PDF，且当前模型能通过 Read 打开 PDF（原生 PDF 或视觉）或任务是创建/加工而非“替无能力模型读懂 PDF”时；需要专业 Typst 排版时不要使用本 Skill。
user-invocable: true
---

# Awaken PDF Skill

用 **Bash + Python 库 / Poppler** 处理通用 PDF。复杂示例见 `references/reference.md`、填表见 `references/forms.md`（按需读取）。专业长文档、Markdown 精排、论文、简历和 Typst/PDF 幻灯片交给 `PDF_Max`（调用 ID：`pdf-max`）。交付走 **`artifact_publish`**。

## 模型能力硬停（阅读任务）

**先 `Read` 目标 PDF，再决定是否继续本 Skill。**

若 `Read` 返回 `model_pdf_unsupported` / `unsupported_media`（当前模型既无原生 PDF、也无视觉）：

1. **立即停止**阅读/摘要/看图类任务；
2. **明确告知用户**：请切换到支持原生 PDF 或视觉（vision）的模型后再试；
3. **禁止**为绕过该限制而：加载本 Skill 的安装剧本、`uv`/`pip`/`venv` 装 `pdfplumber`/`pypdf`、装 Poppler/tesseract/OCR、用 Bash 抽文本或渲图冒充“已读懂”、或改去装 Excel/Word 依赖读同源文件来替代用户点名的 PDF。

本 Skill **不能**把无 PDF/无视觉的模型变成能读 PDF。加工/生成路径（合并、拆分、新建、填表等）仅在任务本身不依赖“当前模型看见 PDF 内容”时继续；若用户要的就是读懂附件 PDF，一律走上面的硬停。

## 阅读分页策略

页数未知时按以下顺序执行：

1. 首次优先显式传 `pages: "1-10"`。如果模型省略 `pages`，宿主也会把超过 10 页的渲染页路线自动归一化为首段 `1-10`，不会先返回分页参数异常；已知不超过 10 页则仍整份读取，`route=native_pdf` 则表示原生整份已完整交给模型。
2. 从成功结果的 `page_count` 与 `route` 判断后续：`route=native_pdf` 表示整份 PDF 已交给模型，无需重复读取；渲染页路线且 `page_count > 10` 时，继续读取 `11-20`、`21-30` 等剩余范围。
3. 每次最多 20 页；已经知道总页数时，最后一段只请求真实剩余页。

模型应为已知 >10 页的后续读取显式传页码；即使首次遗漏，宿主也不会整份渲染长 PDF。该策略保留上下文保护，同时避免把正常的页数发现过程显示成一次工具异常。

## Awaken 工具约定

| 步骤 | 工具 |
| --- | --- |
| 读 PDF 原文、预览图 | **`Read`（唯一阅读入口）** |
| 跑脚本、pdftotext/pdftoppm（仅加工/生成，且模型已能读或任务不需读懂） | `Bash` |
| 安装 Poppler 等系统级依赖（仅加工/生成缺工具时） | `dependency_install`（优先），或 `Bash` |
| 改脚本 | `Edit` / `Write` |
| 找文件 | `Glob` / `Grep` |
| 最终交付 | **`artifact_publish`** |

宿主会按当前模型路由原生 PDF 或 Poppler 渲图。`Read` 已覆盖的阅读不要改用 Bash。本 Skill 的价值在于 `Read` 覆盖不了的**加工**：抽表格、合并拆分、填表、加水印、生成新 PDF——且前提是能力硬停未触发。

**重要**：最终 `.pdf` 必须生成在任务工作区（output roots）内，`artifact_publish` 不接受工作区外的路径。

## 依赖（仅加工/生成）

阅读被能力硬停时不要执行本节。

```bash
# Python 库：新 macOS/Homebrew Python 受 PEP 668 保护。优先顺序：
#   1) uv pip install pypdf pdfplumber reportlab pandas
#   2) python3 -m venv .venv && .venv/bin/pip install pypdf pdfplumber reportlab pandas
#   3) pip3 install --user --break-system-packages pypdf pdfplumber reportlab pandas
python3 -c "import pypdf, pdfplumber" 2>/dev/null || echo "need pypdf pdfplumber"

# 命令行工具（Poppler）：缺失优先用 dependency_install 安装
which pdftotext; which pdftoppm
# OCR（可选）: tesseract / ocrmypdf
```

Windows 上把 `python3` / `pip3` 换成 `python` / `pip`（或 `py -m pip`）。

## 快速对照

| 任务 | 推荐 |
| --- | --- |
| 读懂/摘要 PDF | **`Read`**；页数未知先传 `pages: "1-10"`；若 `model_pdf_unsupported` → 请用户换模型 |
| 抽表格（模型已能读或任务为加工） | `pdfplumber` → DataFrame / Excel |
| 合并/拆分/旋转/元数据 | `pypdf` |
| 新建 PDF | `reportlab` |
| 填可填表单 | 见 `references/forms.md` + `scripts/` |
| 页预览 | `Read`；或加工时用 `pdftoppm` |
| Typst 专业长文档/PDF 幻灯片 | 改用 `PDF_Max` Skill（调用 ID：`pdf-max`） |
| 交付 | `artifact_publish` |

## 阅读（仅当 Read 已成功）

模型已通过 `Read` 看到内容后，如需结构化抽表再使用：

```python
import pdfplumber
with pdfplumber.open("document.pdf") as pdf:
    for page in pdf.pages:
        print(page.extract_text())
        for table in page.extract_tables() or []:
            print(table)
```

```bash
pdftotext -layout input.pdf output.txt
pdftoppm -jpeg -r 120 input.pdf preview/page
```

## 合并 / 拆分（pypdf）

```python
from pypdf import PdfReader, PdfWriter

writer = PdfWriter()
for path in ["a.pdf", "b.pdf"]:
    for page in PdfReader(path).pages:
        writer.add_page(page)
with open("merged.pdf", "wb") as f:
    writer.write(f)
```

## 新建（reportlab）

```python
from reportlab.lib.pagesizes import A4
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer
from reportlab.lib.styles import getSampleStyleSheet
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont

# 中文请注册本机字体，例如：
# pdfmetrics.registerFont(TTFont("YaHei", "/path/to/msyh.ttf"))

doc = SimpleDocTemplate("report.pdf", pagesize=A4)
styles = getSampleStyleSheet()
story = [Paragraph("报告标题", styles["Title"]), Spacer(1, 12),
         Paragraph("正文内容。", styles["Normal"])]
doc.build(story)
```

**重要**：上下标不要用 Unicode 下标字形（易变成黑块），在 Paragraph 里用 `<sub>` / `<super>`。

## 填表

先读 `references/forms.md`，再用：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/extract_form_field_info.py" form.pdf
python3 "${AWAKEN_SKILL_DIR}/scripts/fill_fillable_fields.py" form.pdf values.json output.pdf
```

## 交付检查清单

- [ ] 阅读任务：`Read` 已成功，或已因 `model_pdf_unsupported` 请用户换模型（未装包绕过）
- [ ] 文本可抽或视觉预览可接受（加工/生成）
- [ ] 中文已用本机字体嵌入（新建时）
- [ ] 最终文件位于任务工作区内
- [ ] 已 **`artifact_publish`**

## 明确不做

- 不在无原生 PDF 且无视觉的模型上用本 Skill 或 Bash 冒充阅读能力。
- 不假设本机已预装 Poppler/OCR；加工/生成缺失时优先 `dependency_install`，装不上则如实说明。
- 未安装 OCR 时不假装已 OCR。
