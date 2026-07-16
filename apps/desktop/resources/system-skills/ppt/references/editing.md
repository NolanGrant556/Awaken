# 编辑已有 PPTX（Awaken）

## 优先路径：python-pptx

适合改文案、换图、增删页，不必解包 XML。

```bash
# PEP 668 环境优先 uv / venv，见 SKILL.md 依赖一节
python3 -c "import pptx" 2>/dev/null || echo "need python-pptx"
```

```python
from pptx import Presentation

prs = Presentation("template.pptx")
for i, slide in enumerate(prs.slides):
    for shape in slide.shapes:
        if not shape.has_text_frame:
            continue
        for p in shape.text_frame.paragraphs:
            for run in p.runs:
                if "占位标题" in run.text:
                    run.text = run.text.replace("占位标题", "真实标题")
prs.save("output.pptx")
```

## 从模板创建

1. 用 `python3 -m markitdown template.pptx` 摸清占位文案。
2. 用 `${AWAKEN_SKILL_DIR}/scripts/render_slides.py` 看版式。
3. 用 python-pptx 替换文本/图片，尽量保留母版与占位符几何。
4. 再渲图，用 Awaken `Read` 做视觉 QA。

## 低层路径（少用）

仅当必须改 OOXML 关系或母版时：把 pptx 当 zip 解压 → 改 XML → 重新打包。改完务必用渲染脚本验证。
