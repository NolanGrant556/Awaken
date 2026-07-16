---
name: pdf
version: 1.0.0
description: 处理 PDF：阅读/抽取文本与表格、合并拆分、旋转、水印、新建、填表、加密解密、抽图，以及扫描件 OCR（本机有工具时）。用户提到 pdf 或要产出 PDF 时使用。
when_to_use: 需要读写或生成 PDF 时；需要合并拆分或填表时；需要把内容做成可交付 PDF 并发布到 Workbench 时。
user-invocable: true
---

# Awaken PDF Skill

用 **Bash + Python 库 / Poppler** 处理 PDF。复杂示例见 `references/reference.md`、填表见 `references/forms.md`（按需读取）。交付走 **`artifact_publish`**。

## Awaken 工具约定

| 步骤 | 工具 |
| --- | --- |
| 跑脚本、pdftotext/pdftoppm | `Bash` |
| 安装 Poppler / OCR 等系统级依赖 | `dependency_install`（优先），或 `Bash` |
| 读 PDF 原文、抽取文本、预览图 | `Read`（PDF 路由由宿主自动选择原生或渲图） |
| 改脚本 | `Edit` / `Write` |
| 找文件 | `Glob` / `Grep` |
| 最终交付 | **`artifact_publish`** |

读 PDF 内容直接用 `Read` 即可，不需要判断模型能力；宿主会按当前模型自动路由。本 Skill 的价值在于 `Read` 覆盖不了的加工：抽表格、合并拆分、填表、加水印、生成新 PDF。

**重要**：最终 `.pdf` 必须生成在任务工作区（output roots）内，`artifact_publish` 不接受工作区外的路径。

## 依赖

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
| 抽文本/版面 | `pdfplumber` 或 `pdftotext -layout` |
| 抽表格 | `pdfplumber` → DataFrame / Excel |
| 合并/拆分/旋转/元数据 | `pypdf` |
| 新建 PDF | `reportlab` |
| 填可填表单 | 见 `references/forms.md` + `scripts/` |
| 页预览 | `pdftoppm` → `Read` |
| 交付 | `artifact_publish` |

## 阅读

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

- [ ] 文本可抽或视觉预览可接受
- [ ] 中文已用本机字体嵌入（新建时）
- [ ] 最终文件位于任务工作区内
- [ ] 已 **`artifact_publish`**

## 明确不做

- 不假设本机已预装 Poppler/OCR；缺失时优先 `dependency_install`，装不上则如实说明。
- 未安装 OCR 时不假装已 OCR。
