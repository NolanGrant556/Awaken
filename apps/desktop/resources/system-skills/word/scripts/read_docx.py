#!/usr/bin/env python3
"""Deterministically extract analyzable content from DOCX/DOTX with stdlib only."""

from __future__ import annotations

import argparse
import json
import posixpath
import re
import stat
import sys
import zipfile
from pathlib import Path, PurePosixPath
from urllib.parse import quote, urlsplit
from xml.etree import ElementTree


WORD_NS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
REL_NS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
PACKAGE_REL_NS = "http://schemas.openxmlformats.org/package/2006/relationships"
MATH_NS = "http://schemas.openxmlformats.org/officeDocument/2006/math"
DRAWING_WORD_NS = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
CORE_NS = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
DC_NS = "http://purl.org/dc/elements/1.1/"
DCTERMS_NS = "http://purl.org/dc/terms/"
MC_NS = "http://schemas.openxmlformats.org/markup-compatibility/2006"

REQUIRED_PARTS = {"[Content_Types].xml", "_rels/.rels", "word/document.xml"}
MAX_ENTRIES = 20_000
MAX_TOTAL_UNCOMPRESSED_BYTES = 512 * 1024 * 1024
MAX_XML_BYTES = 32 * 1024 * 1024


def qname(namespace: str, local: str) -> str:
    return f"{{{namespace}}}{local}"


def local_name(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def natural_key(value: str) -> tuple:
    return tuple(int(piece) if piece.isdigit() else piece for piece in re.split(r"(\d+)", value))


class DocxReadError(ValueError):
    """Raised when the package cannot be read safely or deterministically."""


class WarningCollector:
    def __init__(self) -> None:
        self._items: dict[tuple[str, str, str], dict[str, str]] = {}

    def add(self, code: str, message: str, part: str = "") -> None:
        key = (code, part, message)
        item = {"code": code, "message": message}
        if part:
            item["part"] = part
        self._items[key] = item

    def items(self) -> list[dict[str, str]]:
        return [self._items[key] for key in sorted(self._items)]


def canonical_member(name: str) -> str | None:
    if not name or name.startswith("/") or "\\" in name:
        return None
    path = PurePosixPath(name.rstrip("/"))
    if any(part in ("", ".", "..") for part in path.parts):
        return None
    return path.as_posix()


def inspect_package(package: zipfile.ZipFile) -> set[str]:
    infos = package.infolist()
    if len(infos) > MAX_ENTRIES:
        raise DocxReadError(f"OOXML 部件过多: {len(infos)} > {MAX_ENTRIES}")

    names: set[str] = set()
    total_size = 0
    for info in infos:
        name = canonical_member(info.filename)
        if name is None:
            raise DocxReadError(f"非法 OOXML 部件路径: {info.filename!r}")
        if name in names:
            raise DocxReadError(f"重复 OOXML 部件: {name}")
        names.add(name)
        mode = info.external_attr >> 16
        kind = stat.S_IFMT(mode) if mode else 0
        if kind not in (0, stat.S_IFREG, stat.S_IFDIR):
            raise DocxReadError(f"OOXML 包含链接或特殊部件: {name}")
        total_size += info.file_size
        if total_size > MAX_TOTAL_UNCOMPRESSED_BYTES:
            raise DocxReadError(
                "OOXML 解压后体积超过安全上限: "
                f"{total_size} > {MAX_TOTAL_UNCOMPRESSED_BYTES}"
            )

    missing = sorted(REQUIRED_PARTS - names)
    if missing:
        raise DocxReadError(f"DOCX 缺少必需部件: {', '.join(missing)}")
    return names


def parse_xml_bytes(content: bytes, part: str) -> ElementTree.Element:
    if len(content) > MAX_XML_BYTES:
        raise DocxReadError(f"XML 部件超过安全上限: {part}")
    upper = content.upper()
    if b"<!DOCTYPE" in upper or b"<!ENTITY" in upper:
        raise DocxReadError(f"XML 部件包含 DTD/ENTITY: {part}")
    try:
        return ElementTree.fromstring(content)
    except (ElementTree.ParseError, UnicodeError) as error:
        raise DocxReadError(f"XML 无法解析 {part}: {error}") from error


def parse_part(package: zipfile.ZipFile, part: str) -> ElementTree.Element:
    try:
        info = package.getinfo(part)
    except KeyError as error:
        raise DocxReadError(f"OOXML 部件不存在: {part}") from error
    if info.file_size > MAX_XML_BYTES:
        raise DocxReadError(f"XML 部件超过安全上限: {part}")
    try:
        content = package.read(info)
    except (RuntimeError, NotImplementedError, OSError) as error:
        raise DocxReadError(f"OOXML 部件无法读取 {part}: {error}") from error
    return parse_xml_bytes(content, part)


def relationship_part(source_part: str) -> str:
    directory, filename = posixpath.split(source_part)
    return posixpath.join(directory, "_rels", f"{filename}.rels")


def relationships_for(
    package: zipfile.ZipFile, names: set[str], source_part: str, warnings: WarningCollector
) -> dict[str, dict[str, str]]:
    part = relationship_part(source_part)
    if part not in names:
        return {}
    try:
        root = parse_part(package, part)
    except DocxReadError as error:
        warnings.add("relationships_unreadable", str(error), part)
        return {}
    relationships: dict[str, dict[str, str]] = {}
    for relation in root.findall(qname(PACKAGE_REL_NS, "Relationship")):
        relation_id = relation.attrib.get("Id", "")
        if not relation_id:
            continue
        relationships[relation_id] = {
            "target": relation.attrib.get("Target", ""),
            "mode": relation.attrib.get("TargetMode", ""),
            "type": relation.attrib.get("Type", ""),
        }
    return relationships


class DocxExtractor:
    def __init__(self, package: zipfile.ZipFile, names: set[str]) -> None:
        self.package = package
        self.names = names
        self.warnings = WarningCollector()
        self.styles: dict[str, dict[str, str | int | None]] = {}
        self.number_formats: dict[tuple[str, str], str] = {}
        self.number_instances: dict[str, str] = {}
        self._relationship_cache: dict[str, dict[str, dict[str, str]]] = {}
        self.stats = {
            "paragraphs": 0,
            "tables": 0,
            "table_rows": 0,
            "hyperlinks": 0,
            "images_or_drawings": 0,
            "footnotes": 0,
            "endnotes": 0,
            "comments": 0,
            "headers": 0,
            "footers": 0,
        }
        self._load_styles()
        self._load_numbering()
        self._inspect_package_features()

    def relationships(self, part: str) -> dict[str, dict[str, str]]:
        cached = self._relationship_cache.get(part)
        if cached is None:
            cached = relationships_for(self.package, self.names, part, self.warnings)
            self._relationship_cache[part] = cached
        return cached

    def _optional_part(self, part: str) -> ElementTree.Element | None:
        if part not in self.names:
            return None
        try:
            return parse_part(self.package, part)
        except DocxReadError as error:
            self.warnings.add("optional_part_unreadable", str(error), part)
            return None

    def _load_styles(self) -> None:
        root = self._optional_part("word/styles.xml")
        if root is None:
            return
        for style in root.findall(qname(WORD_NS, "style")):
            if style.attrib.get(qname(WORD_NS, "type")) != "paragraph":
                continue
            style_id = style.attrib.get(qname(WORD_NS, "styleId"), "")
            if not style_id:
                continue
            name_node = style.find(qname(WORD_NS, "name"))
            outline_node = style.find(f"./{qname(WORD_NS, 'pPr')}/{qname(WORD_NS, 'outlineLvl')}")
            outline: int | None = None
            if outline_node is not None:
                try:
                    outline = int(outline_node.attrib.get(qname(WORD_NS, "val"), "")) + 1
                except ValueError:
                    outline = None
            self.styles[style_id] = {
                "name": name_node.attrib.get(qname(WORD_NS, "val"), "") if name_node is not None else "",
                "outline": outline,
            }

    def _load_numbering(self) -> None:
        root = self._optional_part("word/numbering.xml")
        if root is None:
            return
        for abstract in root.findall(qname(WORD_NS, "abstractNum")):
            abstract_id = abstract.attrib.get(qname(WORD_NS, "abstractNumId"), "")
            for level in abstract.findall(qname(WORD_NS, "lvl")):
                level_id = level.attrib.get(qname(WORD_NS, "ilvl"), "0")
                format_node = level.find(qname(WORD_NS, "numFmt"))
                number_format = (
                    format_node.attrib.get(qname(WORD_NS, "val"), "")
                    if format_node is not None
                    else ""
                )
                self.number_formats[(abstract_id, level_id)] = number_format
        for number in root.findall(qname(WORD_NS, "num")):
            number_id = number.attrib.get(qname(WORD_NS, "numId"), "")
            abstract_node = number.find(qname(WORD_NS, "abstractNumId"))
            if number_id and abstract_node is not None:
                self.number_instances[number_id] = abstract_node.attrib.get(qname(WORD_NS, "val"), "")

    def _inspect_package_features(self) -> None:
        feature_prefixes = (
            ("word/embeddings/", "embedded_objects_not_interpreted", "嵌入对象未解析"),
            ("word/charts/", "charts_not_interpreted", "图表未解析，只保留正文中的占位提示"),
            ("word/diagrams/", "diagrams_not_interpreted", "SmartArt/diagram 未解析"),
            ("word/activeX/", "activex_not_interpreted", "ActiveX 内容未解析"),
        )
        for prefix, code, message in feature_prefixes:
            if any(name.startswith(prefix) for name in self.names):
                self.warnings.add(code, message, prefix.rstrip("/"))

    def heading_level(self, paragraph: ElementTree.Element) -> int | None:
        properties = paragraph.find(qname(WORD_NS, "pPr"))
        if properties is None:
            return None
        style_node = properties.find(qname(WORD_NS, "pStyle"))
        style_id = style_node.attrib.get(qname(WORD_NS, "val"), "") if style_node is not None else ""
        style = self.styles.get(style_id, {})
        outline = style.get("outline")
        if isinstance(outline, int):
            return max(1, min(6, outline))
        style_name = str(style.get("name", ""))
        candidate = f"{style_id} {style_name}".strip()
        match = re.search(r"(?:heading|标题)\s*([1-6])", candidate, re.IGNORECASE)
        if match:
            return int(match.group(1))
        if re.search(r"(?:^|\s)title(?:$|\s)|(?:^|\s)标题(?:$|\s)", candidate, re.IGNORECASE):
            return 1
        return None

    def list_prefix(self, paragraph: ElementTree.Element, part: str) -> str:
        properties = paragraph.find(qname(WORD_NS, "pPr"))
        numbering = properties.find(qname(WORD_NS, "numPr")) if properties is not None else None
        if numbering is None:
            return ""
        num_id_node = numbering.find(qname(WORD_NS, "numId"))
        level_node = numbering.find(qname(WORD_NS, "ilvl"))
        num_id = num_id_node.attrib.get(qname(WORD_NS, "val"), "") if num_id_node is not None else ""
        level = level_node.attrib.get(qname(WORD_NS, "val"), "0") if level_node is not None else "0"
        abstract_id = self.number_instances.get(num_id, "")
        number_format = self.number_formats.get((abstract_id, level), "")
        self.warnings.add(
            "numbering_flattened",
            "列表保留有序/无序语义，但不保证原始编号值和多级编号文本完全一致",
            part,
        )
        indent = "  " * min(int(level) if level.isdigit() else 0, 6)
        return f"{indent}{'- ' if number_format == 'bullet' else '1. '}"

    def _textbox_text(self, node: ElementTree.Element, part: str) -> str:
        pieces: list[str] = []
        for content in node.iter(qname(WORD_NS, "txbxContent")):
            for paragraph in content.iter(qname(WORD_NS, "p")):
                text = self.paragraph_text(paragraph, part).strip()
                if text:
                    pieces.append(text)
        return " / ".join(pieces)

    def inline_text(
        self,
        node: ElementTree.Element,
        part: str,
        relationships: dict[str, dict[str, str]],
    ) -> str:
        tag = node.tag
        if tag in (qname(WORD_NS, "t"), qname(WORD_NS, "delText"), qname(MATH_NS, "t")):
            return node.text or ""
        if tag == qname(WORD_NS, "tab"):
            return "\t"
        if tag in (qname(WORD_NS, "br"), qname(WORD_NS, "cr")):
            return "<br>"
        if tag == qname(WORD_NS, "noBreakHyphen"):
            return "-"
        if tag == qname(WORD_NS, "softHyphen"):
            return ""
        if tag in (qname(WORD_NS, "del"), qname(WORD_NS, "moveFrom")):
            self.warnings.add(
                "tracked_changes_resolved",
                "阅读结果包含插入/移动目标文本并忽略删除/移动来源文本；原始修订信息仍在 DOCX 中",
                part,
            )
            return ""
        if tag in (qname(WORD_NS, "ins"), qname(WORD_NS, "moveTo")):
            self.warnings.add(
                "tracked_changes_resolved",
                "阅读结果包含插入/移动目标文本并忽略删除/移动来源文本；原始修订信息仍在 DOCX 中",
                part,
            )
        if tag in (qname(WORD_NS, "instrText"), qname(WORD_NS, "fldChar"), qname(WORD_NS, "fldSimple")):
            self.warnings.add(
                "fields_flattened",
                "域代码未求值；仅保留文档中已有的可见域结果文本",
                part,
            )
            if tag != qname(WORD_NS, "fldSimple"):
                return ""
        if tag in (qname(MATH_NS, "oMath"), qname(MATH_NS, "oMathPara")):
            equation = "".join(text.text or "" for text in node.iter(qname(MATH_NS, "t"))).strip()
            self.warnings.add(
                "equations_flattened",
                "公式仅提取线性文本，未保留二维数学版式",
                part,
            )
            return f"[公式: {equation}]" if equation else "[公式]"
        if tag in (qname(WORD_NS, "drawing"), qname(WORD_NS, "object"), qname(WORD_NS, "pict")):
            self.stats["images_or_drawings"] += 1
            textbox = self._textbox_text(node, part)
            if textbox:
                self.warnings.add(
                    "textboxes_flattened",
                    "文本框内容已线性化，空间位置和阅读顺序可能与页面视觉顺序不同",
                    part,
                )
                return f"[文本框: {textbox}]"
            descriptions: list[str] = []
            for metadata in node.iter(qname(DRAWING_WORD_NS, "docPr")):
                for attribute in ("descr", "title", "name"):
                    value = metadata.attrib.get(attribute, "").strip()
                    if value and value not in descriptions:
                        descriptions.append(value)
            self.warnings.add(
                "media_not_interpreted",
                "图片/绘图未做视觉识别，仅保留可用的替代文本或占位符",
                part,
            )
            return f"[图片: {' / '.join(descriptions)}]" if descriptions else "[图片]"
        if tag == qname(WORD_NS, "footnoteReference"):
            note_id = node.attrib.get(qname(WORD_NS, "id"), "?")
            return f"[^脚注-{note_id}]"
        if tag == qname(WORD_NS, "endnoteReference"):
            note_id = node.attrib.get(qname(WORD_NS, "id"), "?")
            return f"[^尾注-{note_id}]"
        if tag == qname(WORD_NS, "commentReference"):
            comment_id = node.attrib.get(qname(WORD_NS, "id"), "?")
            return f"[批注-{comment_id}]"
        if tag == qname(WORD_NS, "altChunk"):
            self.warnings.add(
                "altchunk_not_interpreted",
                "altChunk 外部内容未展开",
                part,
            )
            return "[未展开的 altChunk 内容]"
        if tag == qname(MC_NS, "AlternateContent"):
            fallback = node.find(qname(MC_NS, "Fallback"))
            choices = node.findall(qname(MC_NS, "Choice"))
            selected = fallback if fallback is not None else (choices[0] if choices else None)
            self.warnings.add(
                "alternate_content_selected",
                "兼容性 AlternateContent 只提取 Fallback 或第一个 Choice，未同时解释全部格式分支",
                part,
            )
            if selected is None:
                return ""
            return "".join(self.inline_text(child, part, relationships) for child in selected)

        rendered = "".join(self.inline_text(child, part, relationships) for child in node)
        if tag == qname(WORD_NS, "hyperlink"):
            self.stats["hyperlinks"] += 1
            relation_id = node.attrib.get(qname(REL_NS, "id"), "")
            anchor = node.attrib.get(qname(WORD_NS, "anchor"), "")
            relation = relationships.get(relation_id, {})
            target = relation.get("target", "")
            if target and relation.get("mode", "").lower() == "external":
                scheme = urlsplit(target).scheme.lower()
                has_control = any(ord(character) < 32 for character in target)
                is_hyperlink = relation.get("type", "").rstrip("/").endswith("/hyperlink")
                if scheme in ("http", "https", "mailto") and not has_control and is_hyperlink:
                    safe_target = quote(target, safe=":/?#[]@!$&'(*+,;=%")
                    return f"[{rendered}]({safe_target})" if rendered else safe_target
                self.warnings.add(
                    "unsafe_hyperlink_not_linked",
                    "外链协议、控制字符或关系类型不安全；仅保留显示文本",
                    part,
                )
                return rendered
            if target:
                self.warnings.add(
                    "package_relationship_not_linked",
                    "非 External 的包内关系未转换为 Markdown 链接；仅保留显示文本",
                    part,
                )
                return rendered
            if anchor:
                if re.fullmatch(r"[A-Za-z0-9_.:-]+", anchor):
                    return f"[{rendered}](#{anchor})" if rendered else f"#{anchor}"
                self.warnings.add(
                    "unsafe_bookmark_not_linked",
                    "书签名称不能安全转换为 Markdown 锚点；仅保留显示文本",
                    part,
                )
                return rendered
            self.warnings.add("hyperlink_target_missing", "超链接目标不可用，仅保留显示文本", part)
        return rendered

    def paragraph_text(self, paragraph: ElementTree.Element, part: str) -> str:
        relationships = self.relationships(part)
        text = "".join(
            self.inline_text(child, part, relationships)
            for child in paragraph
            if child.tag != qname(WORD_NS, "pPr")
        )
        return text.strip()

    def paragraph_block(self, paragraph: ElementTree.Element, part: str) -> dict | None:
        self.stats["paragraphs"] += 1
        text = self.paragraph_text(paragraph, part)
        if not text:
            return None
        heading = self.heading_level(paragraph)
        prefix = self.list_prefix(paragraph, part)
        markdown = f"{'#' * heading} {text}" if heading else f"{prefix}{text}"
        return {
            "type": "paragraph",
            "text": text,
            "markdown": markdown,
            **({"heading_level": heading} if heading else {}),
            **({"list": "ordered" if prefix.rstrip().endswith("1.") else "bullet"} if prefix else {}),
        }

    @staticmethod
    def _escape_table_cell(value: str) -> str:
        return value.replace("\\", "\\\\").replace("|", "\\|").replace("\n", "<br>")

    def _cell_text(self, cell: ElementTree.Element, part: str) -> str:
        pieces: list[str] = []
        for child in cell:
            if child.tag == qname(WORD_NS, "p"):
                text = self.paragraph_text(child, part)
                if text:
                    pieces.append(text)
            elif child.tag == qname(WORD_NS, "tbl"):
                self.warnings.add(
                    "nested_tables_flattened",
                    "嵌套表格已线性化到单元格文本",
                    part,
                )
                nested = self.table_rows(child, part)
                pieces.append(" / ".join(" | ".join(row) for row in nested))
            elif child.tag == qname(WORD_NS, "sdt"):
                for paragraph in child.iter(qname(WORD_NS, "p")):
                    text = self.paragraph_text(paragraph, part)
                    if text:
                        pieces.append(text)
        properties = cell.find(qname(WORD_NS, "tcPr"))
        if properties is not None and (
            properties.find(qname(WORD_NS, "gridSpan")) is not None
            or properties.find(qname(WORD_NS, "vMerge")) is not None
        ):
            self.warnings.add(
                "merged_cells_flattened",
                "合并单元格保留文本，但 Markdown/JSON 行列不表达原始跨行跨列关系",
                part,
            )
        return "<br>".join(pieces)

    def table_rows(self, table: ElementTree.Element, part: str) -> list[list[str]]:
        rows: list[list[str]] = []
        for row in table.findall(qname(WORD_NS, "tr")):
            rows.append([self._cell_text(cell, part) for cell in row.findall(qname(WORD_NS, "tc"))])
        return rows

    def table_block(self, table: ElementTree.Element, part: str) -> dict:
        self.stats["tables"] += 1
        rows = self.table_rows(table, part)
        self.stats["table_rows"] += len(rows)
        width = max((len(row) for row in rows), default=0)
        normalized = [row + [""] * (width - len(row)) for row in rows]
        if not width:
            markdown = "[空表格]"
        else:
            headers = [f"列 {index}" for index in range(1, width + 1)]
            lines = [
                "| " + " | ".join(headers) + " |",
                "| " + " | ".join("---" for _ in headers) + " |",
            ]
            lines.extend(
                "| " + " | ".join(self._escape_table_cell(value) for value in row) + " |"
                for row in normalized
            )
            markdown = "\n".join(lines)
        return {"type": "table", "rows": normalized, "markdown": markdown}

    def container_blocks(self, container: ElementTree.Element, part: str) -> list[dict]:
        blocks: list[dict] = []
        for child in container:
            if child.tag == qname(WORD_NS, "p"):
                block = self.paragraph_block(child, part)
                if block:
                    blocks.append(block)
            elif child.tag == qname(WORD_NS, "tbl"):
                blocks.append(self.table_block(child, part))
            elif child.tag == qname(WORD_NS, "sdt"):
                content = child.find(qname(WORD_NS, "sdtContent"))
                if content is not None:
                    blocks.extend(self.container_blocks(content, part))
            elif child.tag == qname(WORD_NS, "altChunk"):
                self.warnings.add("altchunk_not_interpreted", "altChunk 外部内容未展开", part)
                blocks.append(
                    {
                        "type": "unsupported",
                        "text": "未展开的 altChunk 内容",
                        "markdown": "[未展开的 altChunk 内容]",
                    }
                )
            elif child.tag != qname(WORD_NS, "sectPr"):
                text = self.inline_text(child, part, self.relationships(part)).strip()
                if text:
                    self.warnings.add(
                        "body_element_flattened",
                        f"未识别的正文元素 {local_name(child.tag)} 已线性化",
                        part,
                    )
                    blocks.append({"type": "flattened", "text": text, "markdown": text})
        return blocks

    def _notes_section(self, part: str, note_tag: str, label: str, stat: str) -> dict | None:
        root = self._optional_part(part)
        if root is None:
            return None
        notes: list[dict] = []
        for note in root.findall(qname(WORD_NS, note_tag)):
            note_id = note.attrib.get(qname(WORD_NS, "id"), "")
            if note_id in ("-1", "0"):
                continue
            blocks = self.container_blocks(note, part)
            markdown = "\n\n".join(block["markdown"] for block in blocks).strip()
            if markdown:
                notes.append({"id": note_id, "markdown": markdown})
        self.stats[stat] = len(notes)
        if not notes:
            return None
        lines = [f"## {label}"]
        lines.extend(f"[^{label}-{note['id']}]: {note['markdown']}" for note in notes)
        return {"type": stat, "notes": notes, "markdown": "\n".join(lines)}

    def _comments_section(self) -> dict | None:
        part = "word/comments.xml"
        root = self._optional_part(part)
        if root is None:
            return None
        comments: list[dict] = []
        for comment in root.findall(qname(WORD_NS, "comment")):
            comment_id = comment.attrib.get(qname(WORD_NS, "id"), "")
            author = comment.attrib.get(qname(WORD_NS, "author"), "")
            blocks = self.container_blocks(comment, part)
            markdown = "\n\n".join(block["markdown"] for block in blocks).strip()
            if markdown:
                comments.append({"id": comment_id, "author": author, "markdown": markdown})
        self.stats["comments"] = len(comments)
        if not comments:
            return None
        lines = ["## 批注"]
        for comment in comments:
            author = f"（{comment['author']}）" if comment["author"] else ""
            lines.append(f"- 批注-{comment['id']}{author}: {comment['markdown']}")
        return {"type": "comments", "comments": comments, "markdown": "\n".join(lines)}

    def _header_footer_sections(self) -> list[dict]:
        sections: list[dict] = []
        for kind, label, stat in (("header", "页眉", "headers"), ("footer", "页脚", "footers")):
            parts = sorted(
                (
                    name
                    for name in self.names
                    if re.fullmatch(rf"word/{kind}\d+\.xml", name)
                ),
                key=natural_key,
            )
            extracted: list[dict] = []
            for part in parts:
                root = self._optional_part(part)
                if root is None:
                    continue
                blocks = self.container_blocks(root, part)
                markdown = "\n\n".join(block["markdown"] for block in blocks).strip()
                if markdown:
                    extracted.append({"part": part, "markdown": markdown})
            self.stats[stat] = len(extracted)
            if extracted:
                lines = [f"## {label}"]
                lines.extend(f"### {item['part']}\n\n{item['markdown']}" for item in extracted)
                sections.append({"type": stat, "items": extracted, "markdown": "\n\n".join(lines)})
        return sections

    def metadata(self) -> dict[str, str]:
        root = self._optional_part("docProps/core.xml")
        if root is None:
            return {}
        fields = {
            "title": qname(DC_NS, "title"),
            "subject": qname(DC_NS, "subject"),
            "creator": qname(DC_NS, "creator"),
            "description": qname(DC_NS, "description"),
            "keywords": qname(CORE_NS, "keywords"),
            "last_modified_by": qname(CORE_NS, "lastModifiedBy"),
            "created": qname(DCTERMS_NS, "created"),
            "modified": qname(DCTERMS_NS, "modified"),
        }
        result: dict[str, str] = {}
        for key, tag in fields.items():
            node = root.find(tag)
            if node is not None and node.text:
                result[key] = node.text
        return result

    def extract(self) -> dict:
        document = parse_part(self.package, "word/document.xml")
        body = document.find(qname(WORD_NS, "body"))
        if body is None:
            raise DocxReadError("word/document.xml 缺少 w:body")
        blocks = self.container_blocks(body, "word/document.xml")
        footnotes = self._notes_section("word/footnotes.xml", "footnote", "脚注", "footnotes")
        endnotes = self._notes_section("word/endnotes.xml", "endnote", "尾注", "endnotes")
        comments = self._comments_section()
        supplements = [item for item in (footnotes, endnotes, comments) if item]
        supplements.extend(self._header_footer_sections())
        all_blocks = blocks + supplements
        markdown = "\n\n".join(block["markdown"] for block in all_blocks).strip()
        metadata = self.metadata()
        warnings = self.warnings.items()
        return {
            "schema_version": 1,
            "status": "success_with_warnings" if warnings else "success",
            "metadata": metadata,
            "markdown": markdown,
            "blocks": all_blocks,
            "warnings": warnings,
            "stats": self.stats,
        }


def read_docx(path: Path) -> dict:
    resolved = path.expanduser().resolve()
    if resolved.suffix.lower() not in (".docx", ".dotx"):
        raise DocxReadError("只支持 .docx 或 .dotx；旧 .doc 需先转换为 .docx")
    if not resolved.is_file():
        raise DocxReadError(f"文件不存在或不是普通文件: {resolved}")
    try:
        with zipfile.ZipFile(resolved) as package:
            names = inspect_package(package)
            result = DocxExtractor(package, names).extract()
    except zipfile.BadZipFile as error:
        raise DocxReadError(f"不是有效的 OOXML ZIP: {error}") from error
    result["file"] = str(resolved)
    return result


def main() -> int:
    parser = argparse.ArgumentParser(
        description="使用 Python 标准库确定性提取 DOCX/DOTX 正文、表格、注释和页眉页脚"
    )
    parser.add_argument("input", type=Path)
    parser.add_argument("--format", choices=("markdown", "json"), default="markdown")
    parser.add_argument("--output", type=Path, help="写入文件；未提供时输出到 stdout")
    args = parser.parse_args()
    try:
        result = read_docx(args.input)
        text = (
            json.dumps(result, ensure_ascii=False, indent=2)
            if args.format == "json"
            else result["markdown"]
        )
        if args.output:
            output = args.output.expanduser().resolve()
            if output == args.input.expanduser().resolve():
                raise DocxReadError("输出路径不能覆盖输入 DOCX/DOTX")
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_text(text + ("" if text.endswith("\n") else "\n"), encoding="utf-8")
        else:
            print(text)
        if args.format == "markdown":
            for warning in result["warnings"]:
                part = f" ({warning.get('part')})" if warning.get("part") else ""
                print(
                    f"warning[{warning['code']}]{part}: {warning['message']}",
                    file=sys.stderr,
                )
        return 0
    except (DocxReadError, OSError) as error:
        payload = {"status": "failed", "error": str(error)}
        if args.format == "json":
            print(json.dumps(payload, ensure_ascii=False, indent=2))
        else:
            print(f"读取失败: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
