#!/usr/bin/env python3
"""Awaken platform presentation engine.

Models submit semantic slide specs only. This engine owns layout rendering,
CJK font XML helpers, package validation, and incremental edits.
"""

from __future__ import annotations

import json
import os
import sys
import tempfile
import zipfile
from typing import Any

from lxml import etree
from pptx import Presentation
from pptx.dml.color import RGBColor
from pptx.enum.shapes import MSO_SHAPE
from pptx.enum.text import PP_ALIGN
from pptx.oxml.ns import qn
from pptx.util import Emu, Inches, Pt

try:
    from PIL import Image
except Exception:  # pragma: no cover
    Image = None

SUPPORTED_LAYOUTS = (
    "title",
    "section",
    "bullets",
    "two_column",
    "image_right",
    "comparison",
    "metrics",
    "timeline",
    "closing",
)

DEFAULT_THEME = {
    "background": "#F7F9FB",
    "surface": "#FFFFFF",
    "primary": "#12324A",
    "accent": "#0EA5A4",
    "text": "#1F2937",
    "muted": "#64748B",
    "font_latin": "Calibri",
    "font_ea": "PingFang SC",
}

SLIDE_WIDTH = Inches(13.333)
SLIDE_HEIGHT = Inches(7.5)


def rgb(value: Any, fallback: str) -> RGBColor:
    text = str(value or fallback or "").strip()
    if text.startswith("#"):
        text = text[1:]
    if len(text) == 3:
        text = "".join(ch + ch for ch in text)
    if len(text) != 6:
        text = str(fallback or "#000000").strip().lstrip("#")
    try:
        return RGBColor(int(text[0:2], 16), int(text[2:4], 16), int(text[4:6], 16))
    except Exception:
        return RGBColor(0, 0, 0)


def set_run_font(
    run,
    *,
    latin: str | None = None,
    ea: str | None = None,
    cs: str | None = None,
    size_pt: float | None = None,
    bold: bool | None = None,
    color: RGBColor | None = None,
) -> None:
    """Single CJK/Latin font helper used by all text writers.

    XML append uses one node argument for SubElement (parent only via tag).
    """
    if size_pt is not None:
        run.font.size = Pt(size_pt)
    if bold is not None:
        run.font.bold = bold
    if color is not None:
        run.font.color.rgb = color

    r_pr = run._r.get_or_add_rPr()
    for tag in ("a:latin", "a:ea", "a:cs"):
        for existing in list(r_pr.findall(qn(tag))):
            r_pr.remove(existing)

    def append_typeface(tag: str, typeface: str | None) -> None:
        face = str(typeface or "").strip()
        if not face:
            return
        node = etree.SubElement(r_pr, qn(tag))
        node.set("typeface", face)

    append_typeface("a:latin", latin)
    append_typeface("a:ea", ea)
    append_typeface("a:cs", cs or ea)


def set_background(slide, color: str, theme: dict[str, str]) -> None:
    fill = slide.background.fill
    fill.solid()
    fill.fore_color.rgb = rgb(color, theme["background"])


def add_rect(slide, left, top, width, height, color: str, theme: dict[str, str]):
    shape = slide.shapes.add_shape(MSO_SHAPE.RECTANGLE, left, top, width, height)
    shape.fill.solid()
    shape.fill.fore_color.rgb = rgb(color, theme["accent"])
    shape.line.fill.background()
    return shape


def _apply_paragraph_runs(paragraph, text: str, theme: dict[str, str], size: float, bold: bool, color_value: str) -> None:
    paragraph.clear()
    run = paragraph.add_run()
    run.text = text
    set_run_font(
        run,
        latin=theme.get("font_latin"),
        ea=theme.get("font_ea"),
        size_pt=size,
        bold=bold,
        color=rgb(color_value, theme["text"]),
    )


def add_text_box(
    slide,
    left,
    top,
    width,
    height,
    text: Any,
    size: float,
    theme: dict[str, str],
    *,
    bold: bool = False,
    color: str | None = None,
    align=PP_ALIGN.LEFT,
):
    box = slide.shapes.add_textbox(left, top, width, height)
    frame = box.text_frame
    frame.word_wrap = True
    frame.clear()
    lines = str(text or "").splitlines() or [""]
    color_value = color or theme["text"]
    for index, line in enumerate(lines):
        paragraph = frame.paragraphs[0] if index == 0 else frame.add_paragraph()
        paragraph.alignment = align
        paragraph.space_after = Pt(4)
        _apply_paragraph_runs(paragraph, line, theme, size, bold, color_value)
    return box


def add_bullets(slide, left, top, width, height, bullets: list[Any], theme: dict[str, str], *, size: float = 16):
    box = slide.shapes.add_textbox(left, top, width, height)
    frame = box.text_frame
    frame.word_wrap = True
    frame.clear()
    items = [str(item) for item in (bullets or []) if str(item).strip()]
    if not items:
        items = [""]
    for index, item in enumerate(items):
        paragraph = frame.paragraphs[0] if index == 0 else frame.add_paragraph()
        paragraph.alignment = PP_ALIGN.LEFT
        paragraph.space_after = Pt(6)
        _apply_paragraph_runs(paragraph, f"• {item}", theme, size, False, theme["text"])
    return box


def add_body(slide, left, top, width, height, body: Any, bullets: list[Any], theme: dict[str, str]):
    box = slide.shapes.add_textbox(left, top, width, height)
    frame = box.text_frame
    frame.word_wrap = True
    frame.clear()
    first = True
    if body:
        for line in str(body).splitlines():
            paragraph = frame.paragraphs[0] if first else frame.add_paragraph()
            first = False
            paragraph.space_after = Pt(7)
            _apply_paragraph_runs(paragraph, line, theme, 17, False, theme["text"])
    for bullet in bullets or []:
        paragraph = frame.paragraphs[0] if first else frame.add_paragraph()
        first = False
        paragraph.space_after = Pt(6)
        _apply_paragraph_runs(paragraph, f"• {bullet}", theme, 16, False, theme["text"])
    return box


def image_size(path: str):
    if Image is None:
        return None
    try:
        with Image.open(path) as img:
            return img.size
    except Exception:
        return None


def crop_cover(path: str, target_ratio: float, tmpdir: str) -> str:
    if Image is None:
        return path
    try:
        with Image.open(path) as img:
            img = img.convert("RGB")
            width, height = img.size
            current_ratio = float(width) / float(height or 1)
            if current_ratio > target_ratio:
                new_width = int(height * target_ratio)
                left = max(0, int((width - new_width) / 2))
                box = (left, 0, left + new_width, height)
            else:
                new_height = int(width / target_ratio)
                top = max(0, int((height - new_height) / 2))
                box = (0, top, width, top + new_height)
            cropped = img.crop(box)
            out = os.path.join(tmpdir, "crop_" + os.path.basename(path) + ".jpg")
            cropped.save(out, quality=92)
            return out
    except Exception:
        return path


def add_image(slide, item: dict[str, Any], index: int, total: int, theme: dict[str, str], tmpdir: str):
    path = str(item.get("path") or "").strip()
    if not path or not os.path.exists(path):
        return
    default_left = 7.05
    default_top = 1.38 + index * (4.75 / max(1, total))
    default_width = 5.55
    default_height = min(4.55, (4.5 / max(1, total)) - 0.12) if total > 1 else 4.75
    left = float(item.get("x") or default_left)
    top = float(item.get("y") or default_top)
    width = float(item.get("width") or default_width)
    height = float(item.get("height") or default_height)
    fit = str(item.get("fit") or "contain").lower()
    src = crop_cover(path, width / max(0.1, height), tmpdir) if fit == "cover" else path
    size = image_size(src)
    if fit == "stretch" or size is None:
        pic = slide.shapes.add_picture(src, Inches(left), Inches(top), width=Inches(width), height=Inches(height))
    else:
        img_w, img_h = size
        scale = min(width / max(1, img_w), height / max(1, img_h))
        final_w = img_w * scale
        final_h = img_h * scale
        pic = slide.shapes.add_picture(
            src,
            Inches(left + (width - final_w) / 2),
            Inches(top + (height - final_h) / 2),
            width=Inches(final_w),
            height=Inches(final_h),
        )
    if item.get("alt"):
        try:
            pic.name = str(item.get("alt"))
        except Exception:
            pass
    if item.get("caption"):
        add_text_box(
            slide,
            Inches(left),
            Inches(top + height + 0.06),
            Inches(width),
            Inches(0.28),
            item.get("caption"),
            9,
            theme,
            color=theme["muted"],
        )


def add_notes(slide, notes: Any, theme: dict[str, str]) -> None:
    if not notes:
        return
    try:
        frame = slide.notes_slide.notes_text_frame
        frame.clear()
        paragraph = frame.paragraphs[0]
        paragraph.clear()
        run = paragraph.add_run()
        run.text = str(notes)
        set_run_font(
            run,
            latin=theme.get("font_latin"),
            ea=theme.get("font_ea"),
            size_pt=12,
            color=rgb(theme["text"], theme["text"]),
        )
    except Exception:
        pass


def chrome(slide, item: dict[str, Any], theme: dict[str, str]) -> str:
    set_background(slide, item.get("background") or theme["background"], theme)
    accent = item.get("accent") or theme["accent"]
    add_rect(slide, Inches(0), Inches(0), SLIDE_WIDTH, Inches(0.12), accent, theme)
    add_rect(slide, Inches(0.55), Inches(6.95), Inches(12.2), Inches(0.03), accent, theme)
    return accent


def resolve_layout(item: dict[str, Any]) -> str:
    layout = str(item.get("layout") or "").strip()
    if not layout:
        if item.get("images"):
            return "image_right"
        if item.get("bullets"):
            return "bullets"
        if item.get("subtitle") and not item.get("body") and not item.get("bullets"):
            return "section"
        return "bullets"
    if layout not in SUPPORTED_LAYOUTS:
        raise ValueError(
            f"unsupported layout {layout!r}; expected one of: {', '.join(SUPPORTED_LAYOUTS)}"
        )
    return layout


def render_layout(slide, item: dict[str, Any], theme: dict[str, str], tmpdir: str) -> None:
    layout = resolve_layout(item)
    accent = chrome(slide, item, theme)
    title = item.get("title") or ""
    subtitle = item.get("subtitle") or ""
    body = item.get("body") or ""
    bullets = item.get("bullets") or []
    images = item.get("images") or []
    columns = item.get("columns") or []
    metrics = item.get("metrics") or []
    timeline = item.get("timeline") or item.get("steps") or []

    if layout == "title":
        add_text_box(slide, Inches(1.2), Inches(2.4), Inches(10.8), Inches(1.4), title, 40, theme, bold=True, color=theme["primary"], align=PP_ALIGN.CENTER)
        if subtitle:
            add_text_box(slide, Inches(1.8), Inches(4.0), Inches(9.6), Inches(0.8), subtitle, 18, theme, color=theme["muted"], align=PP_ALIGN.CENTER)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "section":
        add_rect(slide, Inches(0), Inches(2.6), SLIDE_WIDTH, Inches(2.2), accent, theme)
        add_text_box(slide, Inches(1.0), Inches(3.0), Inches(11.2), Inches(1.0), title, 34, theme, bold=True, color="#FFFFFF")
        if subtitle:
            add_text_box(slide, Inches(1.0), Inches(4.1), Inches(11.2), Inches(0.5), subtitle, 16, theme, color="#FFFFFF")
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "closing":
        add_text_box(slide, Inches(1.2), Inches(2.6), Inches(10.8), Inches(1.2), title or "Thank you", 36, theme, bold=True, color=theme["primary"], align=PP_ALIGN.CENTER)
        content = body or "\n".join(str(b) for b in bullets)
        if content:
            add_text_box(slide, Inches(2.0), Inches(4.1), Inches(9.2), Inches(1.4), content, 16, theme, color=theme["muted"], align=PP_ALIGN.CENTER)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "two_column":
        add_text_box(slide, Inches(0.62), Inches(0.42), Inches(12.0), Inches(0.7), title, 28, theme, bold=True, color=theme["primary"])
        left_col = columns[0] if len(columns) > 0 else {"title": "Left", "bullets": bullets[: max(1, len(bullets) // 2)] or ([body] if body else [])}
        right_col = columns[1] if len(columns) > 1 else {"title": "Right", "bullets": bullets[max(1, len(bullets) // 2) :] or []}
        for idx, col in enumerate((left_col, right_col)):
            left = 0.7 if idx == 0 else 7.0
            add_text_box(slide, Inches(left), Inches(1.35), Inches(5.4), Inches(0.4), col.get("title") or ("Left" if idx == 0 else "Right"), 18, theme, bold=True, color=theme["accent"])
            add_bullets(slide, Inches(left), Inches(1.9), Inches(5.4), Inches(4.5), col.get("bullets") or [], theme)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "comparison":
        add_text_box(slide, Inches(0.62), Inches(0.42), Inches(12.0), Inches(0.7), title, 28, theme, bold=True, color=theme["primary"])
        left_col = columns[0] if len(columns) > 0 else {"title": "A", "bullets": bullets[: max(1, len(bullets) // 2)]}
        right_col = columns[1] if len(columns) > 1 else {"title": "B", "bullets": bullets[max(1, len(bullets) // 2) :]}
        add_rect(slide, Inches(0.7), Inches(1.4), Inches(5.5), Inches(4.8), theme["surface"], theme)
        add_rect(slide, Inches(7.1), Inches(1.4), Inches(5.5), Inches(4.8), theme["surface"], theme)
        for idx, col in enumerate((left_col, right_col)):
            left = 0.95 if idx == 0 else 7.35
            add_text_box(slide, Inches(left), Inches(1.6), Inches(5.0), Inches(0.4), col.get("title") or ("A" if idx == 0 else "B"), 18, theme, bold=True, color=theme["accent"])
            add_bullets(slide, Inches(left), Inches(2.2), Inches(5.0), Inches(3.7), col.get("bullets") or [], theme)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "metrics":
        add_text_box(slide, Inches(0.62), Inches(0.42), Inches(12.0), Inches(0.7), title, 28, theme, bold=True, color=theme["primary"])
        cards = metrics or [{"label": b, "value": str(i + 1)} for i, b in enumerate(bullets[:4])] or [{"label": "Metric", "value": "—"}]
        count = min(4, max(1, len(cards)))
        card_width = 11.8 / count
        for idx, card in enumerate(cards[:count]):
            left = 0.7 + idx * card_width
            add_rect(slide, Inches(left), Inches(1.8), Inches(card_width - 0.25), Inches(3.6), theme["surface"], theme)
            add_text_box(slide, Inches(left + 0.2), Inches(2.3), Inches(card_width - 0.65), Inches(1.0), str(card.get("value") or ""), 32, theme, bold=True, color=theme["primary"], align=PP_ALIGN.CENTER)
            add_text_box(slide, Inches(left + 0.2), Inches(3.6), Inches(card_width - 0.65), Inches(1.0), str(card.get("label") or ""), 14, theme, color=theme["muted"], align=PP_ALIGN.CENTER)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "timeline":
        add_text_box(slide, Inches(0.62), Inches(0.42), Inches(12.0), Inches(0.7), title, 28, theme, bold=True, color=theme["primary"])
        steps = timeline or [{"title": b} for b in bullets] or [{"title": "Step 1"}, {"title": "Step 2"}]
        count = min(5, max(1, len(steps)))
        add_rect(slide, Inches(1.0), Inches(3.5), Inches(11.2), Inches(0.06), accent, theme)
        span = 11.0 / max(1, count - 1) if count > 1 else 0
        for idx, step in enumerate(steps[:count]):
            left = 1.0 if count == 1 else 1.0 + idx * span
            add_rect(slide, Inches(left), Inches(3.35), Inches(0.35), Inches(0.35), accent, theme)
            add_text_box(slide, Inches(left - 0.6), Inches(3.9), Inches(2.0), Inches(1.4), str(step.get("title") or step.get("label") or f"Step {idx + 1}"), 12, theme, bold=True, color=theme["text"], align=PP_ALIGN.CENTER)
            if step.get("detail") or step.get("body"):
                add_text_box(slide, Inches(left - 0.6), Inches(5.2), Inches(2.0), Inches(1.0), str(step.get("detail") or step.get("body")), 10, theme, color=theme["muted"], align=PP_ALIGN.CENTER)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "image_right":
        title_top = 0.42
        if subtitle:
            add_text_box(slide, Inches(0.62), Inches(0.42), Inches(11.95), Inches(0.25), subtitle, 10, theme, bold=True, color=theme["accent"])
            title_top = 0.68
        add_text_box(slide, Inches(0.62), Inches(title_top), Inches(12.2), Inches(0.72), title, 27, theme, bold=True, color=theme["primary"])
        add_body(slide, Inches(0.72), Inches(1.45), Inches(6.05), Inches(5.25), body, bullets, theme)
        for image_index, image in enumerate(images):
            add_image(slide, image, image_index, len(images), theme, tmpdir)
        add_notes(slide, item.get("notes"), theme)
        return

    # bullets (default explicit layout)
    title_top = 0.42
    if subtitle:
        add_text_box(slide, Inches(0.62), Inches(0.42), Inches(11.95), Inches(0.25), subtitle, 10, theme, bold=True, color=theme["accent"])
        title_top = 0.68
    add_text_box(slide, Inches(0.62), Inches(title_top), Inches(12.2), Inches(0.72), title, 27, theme, bold=True, color=theme["primary"])
    add_body(slide, Inches(0.72), Inches(1.45), Inches(11.9), Inches(5.25), body, bullets, theme)
    add_notes(slide, item.get("notes"), theme)


def clear_slide_shapes(slide) -> None:
    for shape in list(slide.shapes):
        sp = shape._element
        sp.getparent().remove(sp)


def validate_overflow(prs: Presentation) -> list[str]:
    warnings: list[str] = []
    max_w = int(prs.slide_width)
    max_h = int(prs.slide_height)
    for index, slide in enumerate(prs.slides):
        if len(slide.shapes) == 0:
            warnings.append(f"slide[{index}] appears blank")
        for shape in slide.shapes:
            try:
                right = int(shape.left) + int(shape.width)
                bottom = int(shape.top) + int(shape.height)
            except Exception:
                continue
            if right > max_w + Emu(9144) or bottom > max_h + Emu(9144):
                warnings.append(f"slide[{index}] shape overflows slide bounds")
                break
            if getattr(shape, "has_text_frame", False):
                text = shape.text_frame.text if shape.text_frame else ""
                if len(text) > 1200:
                    warnings.append(f"slide[{index}] text may overflow ({len(text)} chars)")
                    break
    return warnings


def save_and_validate_package(prs: Presentation, output_path: str) -> dict[str, Any]:
    parent = os.path.dirname(output_path) or "."
    os.makedirs(parent, exist_ok=True)
    prs.save(output_path)
    if not os.path.exists(output_path) or os.path.getsize(output_path) <= 0:
        raise RuntimeError("generated pptx is empty")
    required = ("[Content_Types].xml", "ppt/presentation.xml")
    with zipfile.ZipFile(output_path, "r") as zf:
        names = set(zf.namelist())
        for item in required:
            if item not in names:
                raise RuntimeError(f"generated pptx missing {item}")
        slide_files = [name for name in names if name.startswith("ppt/slides/slide") and name.endswith(".xml")]
        if not slide_files:
            raise RuntimeError("generated pptx has no slides")
    warnings = validate_overflow(prs)
    return {
        "path": output_path,
        "slides": len(prs.slides),
        "size": os.path.getsize(output_path),
        "warnings": warnings,
    }


def merge_theme(payload: dict[str, Any]) -> dict[str, str]:
    theme = dict(DEFAULT_THEME)
    theme.update({k: v for k, v in (payload.get("theme") or {}).items() if v})
    return theme


def create_presentation(payload: dict[str, Any]) -> dict[str, Any]:
    output_path = payload["output_path"]
    slides = payload.get("slides") or [{"title": "New presentation", "body": "", "layout": "title"}]
    theme = merge_theme(payload)
    prs = Presentation()
    prs.slide_width = SLIDE_WIDTH
    prs.slide_height = SLIDE_HEIGHT
    blank_layout = prs.slide_layouts[6]
    tmpdir = tempfile.mkdtemp(prefix="awaken-ppt-assets-")
    for item in slides:
        slide = prs.slides.add_slide(blank_layout)
        render_layout(slide, item, theme, tmpdir)
    return save_and_validate_package(prs, output_path)


def _slide_count(prs: Presentation) -> int:
    return len(prs.slides)


def apply_edits(payload: dict[str, Any]) -> dict[str, Any]:
    source_path = str(payload.get("source_path") or "").strip()
    if not source_path:
        raise ValueError("source_path is required for presentation_edit")
    edits = payload.get("edits")
    if not isinstance(edits, list) or len(edits) == 0:
        raise ValueError("edits is required and must be a non-empty array")
    output_path = str(payload.get("output_path") or source_path).strip()
    theme = merge_theme(payload)
    prs = Presentation(source_path)
    prs.slide_width = SLIDE_WIDTH
    prs.slide_height = SLIDE_HEIGHT
    blank_layout = prs.slide_layouts[6]
    tmpdir = tempfile.mkdtemp(prefix="awaken-ppt-edit-")

    # Process deletes high-to-low, then inserts/updates in order.
    normalized = []
    for edit in edits:
        if not isinstance(edit, dict):
            raise ValueError("each edit must be an object")
        op = str(edit.get("op") or "update").strip().lower()
        if op not in {"update", "replace", "delete", "insert"}:
            raise ValueError(f"unsupported edit op {op!r}; expected update|replace|delete|insert")
        if "slide_index" not in edit:
            raise ValueError("each edit requires slide_index")
        slide_index = int(edit["slide_index"])
        normalized.append((op, slide_index, edit))

    for op, slide_index, edit in sorted(
        [item for item in normalized if item[0] == "delete"],
        key=lambda item: item[1],
        reverse=True,
    ):
        if slide_index < 0 or slide_index >= _slide_count(prs):
            raise ValueError(f"slide_index {slide_index} out of range")
        rId = prs.slides._sldIdLst[slide_index].rId
        prs.part.drop_rel(rId)
        del prs.slides._sldIdLst[slide_index]

    for op, slide_index, edit in [item for item in normalized if item[0] != "delete"]:
        if op == "insert":
            slide_spec = edit.get("slide")
            if not isinstance(slide_spec, dict):
                raise ValueError("insert edit requires slide object")
            if slide_index < 0 or slide_index > _slide_count(prs):
                raise ValueError(f"slide_index {slide_index} out of range for insert")
            slide = prs.slides.add_slide(blank_layout)
            render_layout(slide, slide_spec, theme, tmpdir)
            # Move newly added slide to requested index.
            sldIdLst = prs.slides._sldIdLst
            sldId = sldIdLst[-1]
            sldIdLst.remove(sldId)
            sldIdLst.insert(slide_index, sldId)
            continue

        if slide_index < 0 or slide_index >= _slide_count(prs):
            raise ValueError(f"slide_index {slide_index} out of range")
        slide = prs.slides[slide_index]
        slide_spec = edit.get("slide")
        if not isinstance(slide_spec, dict):
            # Field-level update: merge onto a minimal slide spec.
            slide_spec = {
                "title": edit.get("title"),
                "subtitle": edit.get("subtitle"),
                "body": edit.get("body"),
                "bullets": edit.get("bullets"),
                "notes": edit.get("notes"),
                "layout": edit.get("layout") or "bullets",
                "images": edit.get("images"),
                "columns": edit.get("columns"),
                "metrics": edit.get("metrics"),
                "timeline": edit.get("timeline"),
                "background": edit.get("background"),
                "accent": edit.get("accent"),
            }
            slide_spec = {k: v for k, v in slide_spec.items() if v is not None}
        clear_slide_shapes(slide)
        render_layout(slide, slide_spec, theme, tmpdir)

    return save_and_validate_package(prs, output_path)


def main() -> int:
    payload = json.load(sys.stdin)
    mode = str(payload.get("mode") or "create").strip().lower()
    try:
        if mode == "edit":
            result = apply_edits(payload)
        else:
            # Validate layouts before mutating files.
            for index, item in enumerate(payload.get("slides") or []):
                if isinstance(item, dict) and item.get("layout"):
                    resolve_layout(item)
            result = create_presentation(payload)
    except Exception as exc:
        print(json.dumps({"error": str(exc)}), file=sys.stderr)
        return 1
    print(json.dumps(result))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
