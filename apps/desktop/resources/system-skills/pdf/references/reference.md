# PDF 进阶参考（Awaken）

## 命令行（Poppler）

```bash
pdftotext -layout in.pdf out.txt
pdftoppm -jpeg -r 150 in.pdf page
pdfinfo in.pdf
```

## 水印 / 叠字（reportlab + pypdf 思路）

1. 用 reportlab 画一页透明水印 PDF。
2. 用 pypdf 把水印 merge 到每一页。
（具体坐标按页尺寸调整。）

## OCR（可选）

本机有 `ocrmypdf` / `tesseract` 时：

```bash
ocrmypdf input.pdf searchable.pdf
```

未安装则明确告知用户，不要假装已 OCR。

## 加密

```python
from pypdf import PdfReader, PdfWriter
reader = PdfReader("in.pdf")
writer = PdfWriter()
for p in reader.pages:
    writer.add_page(p)
writer.encrypt("userpass")
with open("out.pdf", "wb") as f:
    writer.write(f)
```

## 与 Awaken Read 的关系

- 读 PDF 内容直接用 `Read`；原生 PDF 或渲图路由由宿主按当前模型自动选择，不需要在 Skill 内判断。
- 本参考里的 `pdftotext` / `pdfplumber` 适用于需要把文本落盘再加工（清洗、抽表、拼接）的场景。
