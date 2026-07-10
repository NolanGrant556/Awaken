#!/usr/bin/env python3
"""Awaken platform presentation engine.

Models submit semantic slide specs only. This engine owns layout rendering,
CJK font XML helpers, package validation, and incremental edits.
"""

from __future__ import annotations

import json
import os
import re
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


class PresentationValidationError(ValueError):
    """A model-input error that must be surfaced as a structured validation outcome."""

    def __init__(self, path: str, reason: str, *, code: str = "validation_error"):
        self.path = path
        self.reason = reason
        self.code = code
        super().__init__(f"{path} {reason}")


CONTENT_FIELDS = {"subtitle", "body", "bullets", "images", "columns", "metrics", "timeline"}
LAYOUT_CONTENT_FIELDS = {
    "title": {"subtitle", "body"},
    "section": {"subtitle", "body"},
    "bullets": {"subtitle", "body", "bullets"},
    "two_column": {"subtitle", "body", "bullets", "columns"},
    "image_right": {"subtitle", "body", "bullets", "images"},
    "comparison": {"subtitle", "body", "bullets", "columns"},
    "metrics": {"subtitle", "body", "bullets", "metrics"},
    "timeline": {"subtitle", "body", "bullets", "timeline"},
    "closing": {"subtitle", "body", "bullets"},
}


def meaningful(value: Any) -> bool:
    if value is None:
        return False
    if isinstance(value, str):
        return bool(value.strip())
    if isinstance(value, (list, dict, tuple)):
        return len(value) > 0
    return True


def validate_color(value: Any, path: str) -> None:
    if not meaningful(value):
        return
    text = str(value).strip()
    if not re.fullmatch(r"#?(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})", text):
        raise PresentationValidationError(path, "must be a 3- or 6-digit hexadecimal color")


def validate_image_spec(image: Any, path: str) -> None:
    if not isinstance(image, dict):
        raise PresentationValidationError(path, "must be an object")
    image_path = str(image.get("path") or "").strip()
    if not image_path:
        raise PresentationValidationError(f"{path}.path", "is required")
    fit = str(image.get("fit") or "contain").strip().lower()
    if fit not in {"contain", "cover", "stretch"}:
        raise PresentationValidationError(f"{path}.fit", f"has unsupported value {fit!r}", code="unsupported_field")
    if fit in {"contain", "cover"}:
        if Image is None:
            raise PresentationValidationError(f"{path}.fit", f"{fit!r} requires Pillow image support", code="unsupported_field")
        if os.path.isfile(image_path) and image_size(image_path) is None:
            raise PresentationValidationError(f"{path}.path", f"cannot be decoded for fit={fit!r}")
    for key in ("x", "y"):
        if key in image and float(image[key]) < 0:
            raise PresentationValidationError(f"{path}.{key}", "must be greater than or equal to 0")
    for key in ("width", "height"):
        if key in image and float(image[key]) <= 0:
            raise PresentationValidationError(f"{path}.{key}", "must be greater than 0")


def validate_slide_spec(item: Any, path: str) -> str:
    if not isinstance(item, dict):
        raise PresentationValidationError(path, "must be an object")
    layout = resolve_layout(item, path)
    allowed = LAYOUT_CONTENT_FIELDS[layout]
    for field in sorted(CONTENT_FIELDS - allowed):
        if meaningful(item.get(field)):
            raise PresentationValidationError(
                f"{path}.{field}",
                f"is unsupported for layout {layout!r}",
                code="unsupported_field",
            )

    columns = item.get("columns") or []
    metrics = item.get("metrics") or []
    timeline = item.get("timeline") or []
    bullets = item.get("bullets") or []
    if layout == "two_column" and len(columns) > 2:
        raise PresentationValidationError(f"{path}.columns", "supports at most 2 items for layout 'two_column'")
    if layout == "comparison" and len(columns) > 3:
        raise PresentationValidationError(f"{path}.columns", "supports at most 3 items for layout 'comparison'")
    if layout == "metrics" and len(metrics) > 4:
        raise PresentationValidationError(f"{path}.metrics", "supports at most 4 items")
    if layout == "timeline" and len(timeline) > 5:
        raise PresentationValidationError(f"{path}.timeline", "supports at most 5 items")
    if layout == "metrics" and not metrics and len(bullets) > 4:
        raise PresentationValidationError(f"{path}.bullets", "supports at most 4 items when used as metrics fallback")
    if layout == "timeline" and not timeline and len(bullets) > 5:
        raise PresentationValidationError(f"{path}.bullets", "supports at most 5 items when used as timeline fallback")
    structured = {
        "two_column": columns,
        "comparison": columns,
        "metrics": metrics,
        "timeline": timeline,
    }.get(layout)
    if structured and bullets:
        field = "columns" if layout in {"two_column", "comparison"} else layout
        raise PresentationValidationError(
            f"{path}.bullets",
            f"cannot be combined with non-empty {field}; choose one content source",
        )

    validate_color(item.get("background"), f"{path}.background")
    validate_color(item.get("accent"), f"{path}.accent")
    for index, image in enumerate(item.get("images") or []):
        validate_image_spec(image, f"{path}.images[{index}]")
    return layout


def validate_theme(theme: Any, path: str = "$.theme") -> None:
    if not theme:
        return
    if not isinstance(theme, dict):
        raise PresentationValidationError(path, "must be an object")
    for field in ("background", "surface", "primary", "accent", "text", "muted"):
        validate_color(theme.get(field), f"{path}.{field}")


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
    role: str | None = None,
):
    box = slide.shapes.add_textbox(left, top, width, height)
    if role:
        box.name = f"Awaken:{role}"
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


def add_standard_header(
    slide,
    title: Any,
    subtitle: Any,
    body: Any,
    theme: dict[str, str],
) -> float:
    """Render common semantic copy and return the first safe content y-coordinate."""
    add_text_box(
        slide,
        Inches(0.62),
        Inches(0.38),
        Inches(12.0),
        Inches(0.62),
        title,
        27,
        theme,
        bold=True,
        color=theme["primary"],
        role="title",
    )
    cursor = 1.05
    if subtitle:
        add_text_box(
            slide,
            Inches(0.72),
            Inches(cursor),
            Inches(11.9),
            Inches(0.28),
            subtitle,
            10,
            theme,
            bold=True,
            color=theme["accent"],
            role="subtitle",
        )
        cursor += 0.34
    if body:
        add_text_box(
            slide,
            Inches(0.72),
            Inches(cursor),
            Inches(11.9),
            Inches(0.48),
            body,
            11,
            theme,
            color=theme["muted"],
            role="body",
        )
        cursor += 0.56
    return max(1.42, cursor + 0.06)


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
    box.name = "Awaken:body"
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
        raise ValueError("fit='cover' requires Pillow image support")
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


def add_image(slide, item: dict[str, Any], index: int, total: int, theme: dict[str, str], tmpdir: str):
    path = str(item.get("path") or "").strip()
    if not path:
        raise ValueError("image path is required")
    if not os.path.isfile(path):
        raise ValueError(f"image file does not exist: {path}")
    default_left = 7.05
    default_top = 1.38 + index * (4.75 / max(1, total))
    default_width = 5.55
    default_height = min(4.55, (4.5 / max(1, total)) - 0.12) if total > 1 else 4.75
    left = float(default_left if item.get("x") is None else item["x"])
    top = float(default_top if item.get("y") is None else item["y"])
    width = float(default_width if item.get("width") is None else item["width"])
    height = float(default_height if item.get("height") is None else item["height"])
    fit = str(item.get("fit") or "contain").lower()
    src = crop_cover(path, width / max(0.1, height), tmpdir) if fit == "cover" else path
    size = image_size(src)
    if fit == "stretch":
        pic = slide.shapes.add_picture(src, Inches(left), Inches(top), width=Inches(width), height=Inches(height))
    else:
        if size is None:
            raise ValueError(f"image cannot be decoded for fit={fit!r}: {path}")
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
    pic.name = f"Awaken:image:{index}"
    if item.get("alt"):
        # Keep the stable semantic name for incremental edits and persist alt
        # text in the OOXML docPr description instead of silently overwriting it.
        pic._element.nvPicPr.cNvPr.set("descr", str(item.get("alt")))
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


def assign_semantic_shape_names(slide, item: dict[str, Any]) -> None:
    """Tag engine-owned shapes so update edits can be field-level and lossless."""
    title = str(item.get("title") or "")
    subtitle = str(item.get("subtitle") or "")
    body = str(item.get("body") or "")
    bullets = [str(value) for value in (item.get("bullets") or [])]
    captions = [str(image.get("caption") or "") for image in (item.get("images") or []) if isinstance(image, dict)]
    picture_index = 0
    for shape in slide.shapes:
        if hasattr(shape, "image"):
            shape.name = f"Awaken:image:{picture_index}"
            picture_index += 1
            continue
        if not getattr(shape, "has_text_frame", False):
            continue
        text = str(shape.text_frame.text or "")
        if title and text == title:
            shape.name = "Awaken:title"
            continue
        if subtitle and text == subtitle:
            shape.name = "Awaken:subtitle"
            continue
        if body and body in text:
            shape.name = "Awaken:body"
            continue
        if bullets and any(value and value in text for value in bullets):
            shape.name = "Awaken:body"
            continue
        for index, caption in enumerate(captions):
            if caption and text == caption:
                shape.name = f"Awaken:image_caption:{index}"
                break


def find_semantic_shape(slide, role: str):
    expected = f"Awaken:{role}"
    for shape in slide.shapes:
        if str(getattr(shape, "name", "")) == expected:
            return shape
    return None


def replace_text_preserving_style(shape, value: Any, theme: dict[str, str], *, fallback_size: float, fallback_bold: bool = False) -> None:
    frame = shape.text_frame
    first_run = None
    for paragraph in frame.paragraphs:
        if paragraph.runs:
            first_run = paragraph.runs[0]
            break
    size = first_run.font.size.pt if first_run is not None and first_run.font.size is not None else fallback_size
    bold = first_run.font.bold if first_run is not None and first_run.font.bold is not None else fallback_bold
    frame.clear()
    lines = str(value or "").splitlines() or [""]
    for index, line in enumerate(lines):
        paragraph = frame.paragraphs[0] if index == 0 else frame.add_paragraph()
        _apply_paragraph_runs(paragraph, line, theme, size, bool(bold), theme["text"])


def update_body_shape(shape, fields: dict[str, Any], theme: dict[str, str]) -> None:
    existing_lines = []
    existing_bullets = []
    for paragraph in shape.text_frame.paragraphs:
        text = str(paragraph.text or "")
        if text.startswith("• "):
            existing_bullets.append(text[2:])
        elif text:
            existing_lines.append(text)
    body = str(fields.get("body")) if "body" in fields else "\n".join(existing_lines)
    bullets = [str(item) for item in fields.get("bullets", existing_bullets)]
    frame = shape.text_frame
    frame.clear()
    first = True
    if body:
        for line in body.splitlines():
            paragraph = frame.paragraphs[0] if first else frame.add_paragraph()
            first = False
            paragraph.space_after = Pt(7)
            _apply_paragraph_runs(paragraph, line, theme, 17, False, theme["text"])
    for bullet in bullets:
        paragraph = frame.paragraphs[0] if first else frame.add_paragraph()
        first = False
        paragraph.space_after = Pt(6)
        _apply_paragraph_runs(paragraph, f"• {bullet}", theme, 16, False, theme["text"])


def update_notes(slide, value: Any, theme: dict[str, str]) -> None:
    frame = slide.notes_slide.notes_text_frame
    frame.clear()
    if value:
        paragraph = frame.paragraphs[0]
        _apply_paragraph_runs(paragraph, str(value), theme, 12, False, theme["text"])


def update_slide_in_place(
    slide,
    fields: dict[str, Any],
    theme: dict[str, str],
    tmpdir: str,
    *,
    path: str,
) -> None:
    structural = sorted(key for key in ("layout", "columns", "metrics", "timeline", "background", "accent") if key in fields)
    if structural:
        raise PresentationValidationError(
            path,
            "update cannot change structural fields " + ", ".join(structural) + "; use op=replace with a complete slide",
            code="unsupported_edit",
        )
    if "title" in fields:
        shape = find_semantic_shape(slide, "title")
        if shape is None:
            raise PresentationValidationError(f"{path}.title", "cannot locate a semantic title shape; use op=replace with a complete slide", code="unsupported_edit")
        replace_text_preserving_style(shape, fields["title"], theme, fallback_size=27, fallback_bold=True)
        shape.name = "Awaken:title"
    if "subtitle" in fields:
        shape = find_semantic_shape(slide, "subtitle")
        if shape is None:
            raise PresentationValidationError(f"{path}.subtitle", "cannot locate a semantic subtitle shape; use op=replace with a complete slide", code="unsupported_edit")
        replace_text_preserving_style(shape, fields["subtitle"], theme, fallback_size=10, fallback_bold=True)
        shape.name = "Awaken:subtitle"
    if "body" in fields or "bullets" in fields:
        shape = find_semantic_shape(slide, "body")
        if shape is None:
            raise PresentationValidationError(f"{path}.body", "cannot locate a semantic body shape; use op=replace with a complete slide", code="unsupported_edit")
        update_body_shape(shape, fields, theme)
        shape.name = "Awaken:body"
    if "images" in fields:
        for shape in list(slide.shapes):
            name = str(getattr(shape, "name", ""))
            if name.startswith("Awaken:image:") or name.startswith("Awaken:image_caption:"):
                element = shape._element
                element.getparent().remove(element)
        images = fields.get("images") or []
        for image_index, image in enumerate(images):
            add_image(slide, image, image_index, len(images), theme, tmpdir)
        assign_semantic_shape_names(slide, fields)
    if "notes" in fields:
        update_notes(slide, fields["notes"], theme)


def add_notes(slide, notes: Any, theme: dict[str, str]) -> None:
    if not notes:
        return
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


def chrome(slide, item: dict[str, Any], theme: dict[str, str]) -> str:
    set_background(slide, item.get("background") or theme["background"], theme)
    accent = item.get("accent") or theme["accent"]
    add_rect(slide, Inches(0), Inches(0), SLIDE_WIDTH, Inches(0.12), accent, theme)
    add_rect(slide, Inches(0.55), Inches(6.95), Inches(12.2), Inches(0.03), accent, theme)
    return accent


def resolve_layout(item: dict[str, Any], path: str = "$.slide") -> str:
    layout = str(item.get("layout") or "").strip()
    if not layout:
        if item.get("metrics"):
            return "metrics"
        if item.get("timeline"):
            return "timeline"
        if item.get("columns"):
            return "comparison" if len(item.get("columns") or []) >= 3 else "two_column"
        if item.get("images"):
            return "image_right"
        if item.get("bullets"):
            return "bullets"
        if item.get("subtitle") and not item.get("body") and not item.get("bullets"):
            return "section"
        return "bullets"
    if layout not in SUPPORTED_LAYOUTS:
        raise PresentationValidationError(
            f"{path}.layout",
            f"has unsupported value {layout!r}; expected one of: {', '.join(SUPPORTED_LAYOUTS)}",
            code="unsupported_layout",
        )
    return layout


def render_layout(
    slide,
    item: dict[str, Any],
    theme: dict[str, str],
    tmpdir: str,
    *,
    path: str = "$.slide",
) -> None:
    layout = validate_slide_spec(item, path)
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
        add_text_box(slide, Inches(1.2), Inches(2.05), Inches(10.8), Inches(1.25), title, 40, theme, bold=True, color=theme["primary"], align=PP_ALIGN.CENTER, role="title")
        if subtitle:
            add_text_box(slide, Inches(1.8), Inches(3.55), Inches(9.6), Inches(0.62), subtitle, 18, theme, color=theme["muted"], align=PP_ALIGN.CENTER, role="subtitle")
        if body:
            add_text_box(slide, Inches(2.0), Inches(4.45), Inches(9.3), Inches(1.0), body, 15, theme, color=theme["text"], align=PP_ALIGN.CENTER, role="body")
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "section":
        add_rect(slide, Inches(0), Inches(2.35), SLIDE_WIDTH, Inches(2.85), accent, theme)
        add_text_box(slide, Inches(1.0), Inches(2.7), Inches(11.2), Inches(0.9), title, 34, theme, bold=True, color="#FFFFFF", role="title")
        if subtitle:
            add_text_box(slide, Inches(1.0), Inches(3.7), Inches(11.2), Inches(0.42), subtitle, 16, theme, color="#FFFFFF", role="subtitle")
        if body:
            add_text_box(slide, Inches(1.0), Inches(4.2), Inches(11.2), Inches(0.72), body, 13, theme, color="#FFFFFF", role="body")
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "closing":
        add_text_box(slide, Inches(1.2), Inches(2.15), Inches(10.8), Inches(1.1), title or "Thank you", 36, theme, bold=True, color=theme["primary"], align=PP_ALIGN.CENTER, role="title")
        if subtitle:
            add_text_box(slide, Inches(2.0), Inches(3.35), Inches(9.2), Inches(0.48), subtitle, 16, theme, bold=True, color=theme["accent"], align=PP_ALIGN.CENTER, role="subtitle")
        if body or bullets:
            add_body(slide, Inches(2.0), Inches(4.0), Inches(9.2), Inches(1.75), body, bullets, theme)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "two_column":
        content_top = add_standard_header(slide, title, subtitle, body, theme)
        left_col = columns[0] if len(columns) > 0 else {"title": "Left", "bullets": bullets[: max(1, len(bullets) // 2)]}
        right_col = columns[1] if len(columns) > 1 else {"title": "Right", "bullets": bullets[max(1, len(bullets) // 2) :] or []}
        for idx, col in enumerate((left_col, right_col)):
            left = 0.7 if idx == 0 else 7.0
            add_text_box(slide, Inches(left), Inches(content_top), Inches(5.4), Inches(0.4), col.get("title") or ("Left" if idx == 0 else "Right"), 18, theme, bold=True, color=theme["accent"])
            add_bullets(slide, Inches(left), Inches(content_top + 0.55), Inches(5.4), Inches(max(1.0, 6.35 - content_top)), col.get("bullets") or [], theme)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "comparison":
        content_top = add_standard_header(slide, title, subtitle, body, theme)
        rendered_columns = list(columns)
        if not rendered_columns:
            split = max(1, (len(bullets) + 1) // 2)
            rendered_columns = [
                {"title": "A", "bullets": bullets[:split]},
                {"title": "B", "bullets": bullets[split:]},
            ]
        count = len(rendered_columns)
        gap = 0.25
        total_width = 11.9
        card_width = (total_width - gap * max(0, count - 1)) / max(1, count)
        card_height = max(1.0, 6.35 - content_top)
        for idx, col in enumerate(rendered_columns):
            left = 0.7 + idx * (card_width + gap)
            add_rect(slide, Inches(left), Inches(content_top), Inches(card_width), Inches(card_height), theme["surface"], theme)
            inner_left = left + 0.2
            inner_width = max(0.8, card_width - 0.4)
            add_text_box(slide, Inches(inner_left), Inches(content_top + 0.2), Inches(inner_width), Inches(0.4), col.get("title") or chr(ord("A") + idx), 17 if count < 3 else 15, theme, bold=True, color=theme["accent"])
            add_bullets(slide, Inches(inner_left), Inches(content_top + 0.75), Inches(inner_width), Inches(max(0.6, card_height - 0.95)), col.get("bullets") or [], theme, size=16 if count < 3 else 14)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "metrics":
        content_top = add_standard_header(slide, title, subtitle, body, theme)
        cards = metrics or [{"label": b, "value": str(i + 1)} for i, b in enumerate(bullets)] or [{"label": "Metric", "value": "—"}]
        count = max(1, len(cards))
        card_width = 11.8 / count
        for idx, card in enumerate(cards):
            left = 0.7 + idx * card_width
            add_rect(slide, Inches(left), Inches(content_top), Inches(card_width - 0.25), Inches(max(2.4, 5.7 - content_top)), theme["surface"], theme)
            add_text_box(slide, Inches(left + 0.2), Inches(content_top + 0.5), Inches(card_width - 0.65), Inches(1.0), str(card.get("value") or ""), 32, theme, bold=True, color=theme["primary"], align=PP_ALIGN.CENTER)
            add_text_box(slide, Inches(left + 0.2), Inches(content_top + 1.8), Inches(card_width - 0.65), Inches(1.0), str(card.get("label") or ""), 14, theme, color=theme["muted"], align=PP_ALIGN.CENTER)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "timeline":
        content_top = add_standard_header(slide, title, subtitle, body, theme)
        steps = timeline or [{"title": b} for b in bullets] or [{"title": "Step 1"}, {"title": "Step 2"}]
        count = max(1, len(steps))
        content_left = 0.7
        content_width = 11.9
        cell_width = content_width / count
        line_y = max(3.0, content_top + 1.15)
        first_center = content_left + cell_width / 2
        last_center = content_left + content_width - cell_width / 2
        add_rect(slide, Inches(first_center), Inches(line_y), Inches(max(0.06, last_center - first_center)), Inches(0.06), accent, theme)
        for idx, step in enumerate(steps):
            cell_left = content_left + idx * cell_width
            center = cell_left + cell_width / 2
            text_left = cell_left + 0.05
            text_width = max(0.7, cell_width - 0.1)
            add_rect(slide, Inches(center - 0.175), Inches(line_y - 0.145), Inches(0.35), Inches(0.35), accent, theme)
            add_text_box(slide, Inches(text_left), Inches(line_y + 0.42), Inches(text_width), Inches(0.7), str(step.get("title") or f"Step {idx + 1}"), 12, theme, bold=True, color=theme["text"], align=PP_ALIGN.CENTER)
            if step.get("detail"):
                add_text_box(slide, Inches(text_left), Inches(line_y + 1.13), Inches(text_width), Inches(0.35), str(step.get("detail")), 10, theme, bold=True, color=theme["accent"], align=PP_ALIGN.CENTER)
            if step.get("body"):
                add_text_box(slide, Inches(text_left), Inches(line_y + 1.5), Inches(text_width), Inches(max(0.5, 6.7 - (line_y + 1.5))), str(step.get("body")), 9, theme, color=theme["muted"], align=PP_ALIGN.CENTER)
        add_notes(slide, item.get("notes"), theme)
        return

    if layout == "image_right":
        title_top = 0.42
        if subtitle:
            add_text_box(slide, Inches(0.62), Inches(0.42), Inches(11.95), Inches(0.25), subtitle, 10, theme, bold=True, color=theme["accent"], role="subtitle")
            title_top = 0.68
        add_text_box(slide, Inches(0.62), Inches(title_top), Inches(12.2), Inches(0.72), title, 27, theme, bold=True, color=theme["primary"], role="title")
        add_body(slide, Inches(0.72), Inches(1.45), Inches(6.05), Inches(5.25), body, bullets, theme)
        for image_index, image in enumerate(images):
            add_image(slide, image, image_index, len(images), theme, tmpdir)
        add_notes(slide, item.get("notes"), theme)
        return

    # bullets (default explicit layout)
    title_top = 0.42
    if subtitle:
        add_text_box(slide, Inches(0.62), Inches(0.42), Inches(11.95), Inches(0.25), subtitle, 10, theme, bold=True, color=theme["accent"], role="subtitle")
        title_top = 0.68
    add_text_box(slide, Inches(0.62), Inches(title_top), Inches(12.2), Inches(0.72), title, 27, theme, bold=True, color=theme["primary"], role="title")
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
        has_audience_content = False
        for shape in slide.shapes:
            if hasattr(shape, "image"):
                has_audience_content = True
                break
            if getattr(shape, "has_text_frame", False) and str(shape.text_frame.text or "").strip():
                has_audience_content = True
                break
        if not has_audience_content:
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
    validate_theme(payload.get("theme"))
    theme = merge_theme(payload)
    prs = Presentation()
    prs.slide_width = SLIDE_WIDTH
    prs.slide_height = SLIDE_HEIGHT
    if payload.get("title"):
        prs.core_properties.title = str(payload.get("title"))
    blank_layout = prs.slide_layouts[6]
    tmpdir = tempfile.mkdtemp(prefix="awaken-ppt-assets-")
    for index, item in enumerate(slides):
        slide = prs.slides.add_slide(blank_layout)
        render_layout(slide, item, theme, tmpdir, path=f"$.slides[{index}]")
        assign_semantic_shape_names(slide, item)
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
    validate_theme(payload.get("theme"))
    theme = merge_theme(payload)
    prs = Presentation(source_path)
    prs.slide_width = SLIDE_WIDTH
    prs.slide_height = SLIDE_HEIGHT
    blank_layout = prs.slide_layouts[6]
    tmpdir = tempfile.mkdtemp(prefix="awaken-ppt-edit-")

    # Apply edits exactly in request order. slide_index is evaluated against
    # the current deck after every preceding operation, matching JSON Patch-
    # style list semantics. For example, deleting index 0 from A/B/C leaves
    # B/C, so a following update of index 1 targets C.
    normalized = []
    for edit_index, edit in enumerate(edits):
        if not isinstance(edit, dict):
            raise PresentationValidationError(f"$.edits[{edit_index}]", "must be an object")
        op = str(edit.get("op") or "update").strip().lower()
        if op not in {"update", "replace", "delete", "insert"}:
            raise PresentationValidationError(f"$.edits[{edit_index}].op", f"has unsupported value {op!r}; expected update|replace|delete|insert", code="unsupported_edit")
        if "slide_index" not in edit:
            raise PresentationValidationError(f"$.edits[{edit_index}].slide_index", "is required")
        slide_index = int(edit["slide_index"])
        slide_spec = edit.get("slide")
        if op in {"insert", "replace"}:
            if not isinstance(slide_spec, dict):
                raise PresentationValidationError(f"$.edits[{edit_index}].slide", f"is required for {op}")
            validate_slide_spec(slide_spec, f"$.edits[{edit_index}].slide")
        if op == "update":
            update_fields = dict(slide_spec or {}) if isinstance(slide_spec, dict) else {}
            for key in (
                "title", "subtitle", "body", "bullets", "notes", "layout", "images",
                "columns", "metrics", "timeline", "background", "accent",
            ):
                if key in edit:
                    update_fields[key] = edit[key]
            structural = sorted(key for key in ("layout", "columns", "metrics", "timeline", "background", "accent") if key in update_fields)
            if structural:
                raise PresentationValidationError(
                    f"$.edits[{edit_index}]",
                    "update cannot change structural fields " + ", ".join(structural) + "; use op=replace with a complete slide",
                    code="unsupported_edit",
                )
            for image_index, image in enumerate(update_fields.get("images") or []):
                validate_image_spec(image, f"$.edits[{edit_index}].images[{image_index}]")
        normalized.append((op, slide_index, edit_index, edit))

    for op, slide_index, edit_index, edit in normalized:
        if op == "delete":
            if slide_index < 0 or slide_index >= _slide_count(prs):
                raise PresentationValidationError(f"$.edits[{edit_index}].slide_index", f"value {slide_index} is out of range")
            rId = prs.slides._sldIdLst[slide_index].rId
            prs.part.drop_rel(rId)
            del prs.slides._sldIdLst[slide_index]
            continue

        if op == "insert":
            slide_spec = edit.get("slide")
            if not isinstance(slide_spec, dict):
                raise PresentationValidationError(f"$.edits[{edit_index}].slide", "is required for insert")
            if slide_index < 0 or slide_index > _slide_count(prs):
                raise PresentationValidationError(f"$.edits[{edit_index}].slide_index", f"value {slide_index} is out of range for insert")
            slide = prs.slides.add_slide(blank_layout)
            render_layout(slide, slide_spec, theme, tmpdir, path=f"$.edits[{edit_index}].slide")
            assign_semantic_shape_names(slide, slide_spec)
            # Move newly added slide to requested index.
            sldIdLst = prs.slides._sldIdLst
            sldId = sldIdLst[-1]
            sldIdLst.remove(sldId)
            sldIdLst.insert(slide_index, sldId)
            continue

        if slide_index < 0 or slide_index >= _slide_count(prs):
            raise PresentationValidationError(f"$.edits[{edit_index}].slide_index", f"value {slide_index} is out of range")
        slide = prs.slides[slide_index]
        slide_spec = dict(edit.get("slide") or {}) if isinstance(edit.get("slide"), dict) else {}
        for key in (
            "title", "subtitle", "body", "bullets", "notes", "layout", "images",
            "columns", "metrics", "timeline", "background", "accent",
        ):
            if key in edit:
                slide_spec[key] = edit[key]
        if op == "replace":
            if not slide_spec:
                raise PresentationValidationError(f"$.edits[{edit_index}].slide", "is required for replace")
            clear_slide_shapes(slide)
            render_layout(slide, slide_spec, theme, tmpdir, path=f"$.edits[{edit_index}].slide")
            assign_semantic_shape_names(slide, slide_spec)
            continue
        if not slide_spec:
            raise PresentationValidationError(f"$.edits[{edit_index}]", "update requires at least one editable field")
        update_slide_in_place(slide, slide_spec, theme, tmpdir, path=f"$.edits[{edit_index}]")

    return save_and_validate_package(prs, output_path)


def main() -> int:
    payload = json.load(sys.stdin)
    mode = str(payload.get("mode") or "create").strip().lower()
    try:
        if mode == "edit":
            result = apply_edits(payload)
        else:
            validate_theme(payload.get("theme"))
            # Validate semantic compatibility before creating any slide/file.
            for index, item in enumerate(payload.get("slides") or []):
                validate_slide_spec(item, f"$.slides[{index}]")
            result = create_presentation(payload)
    except PresentationValidationError as exc:
        print(
            json.dumps(
                {
                    "error": str(exc),
                    "error_code": exc.code,
                    "path": exc.path,
                    "reason": exc.reason,
                }
            ),
            file=sys.stderr,
        )
        return 1
    except Exception as exc:
        print(json.dumps({"error": str(exc)}), file=sys.stderr)
        return 1
    print(json.dumps(result))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
