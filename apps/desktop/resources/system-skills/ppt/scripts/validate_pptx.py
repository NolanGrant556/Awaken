#!/usr/bin/env python3
"""校验 PPTX/POTX 的包、关系、页面注册与常见图表结构。"""

from __future__ import annotations

import argparse
import json
import posixpath
import re
import stat
import zipfile
from pathlib import Path, PurePosixPath
from xml.etree import ElementTree


REQUIRED = {
    "[Content_Types].xml",
    "_rels/.rels",
    "ppt/presentation.xml",
    "ppt/_rels/presentation.xml.rels",
}
REL_NS = "http://schemas.openxmlformats.org/package/2006/relationships"
P_NS = "http://schemas.openxmlformats.org/presentationml/2006/main"
A_NS = "http://schemas.openxmlformats.org/drawingml/2006/main"
R_NS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
C_NS = "http://schemas.openxmlformats.org/drawingml/2006/chart"
CT_NS = "http://schemas.openxmlformats.org/package/2006/content-types"


def canonical(name: str) -> str | None:
    if not name or name.startswith("/") or "\\" in name:
        return None
    path = PurePosixPath(name.rstrip("/"))
    if any(part in ("", ".", "..") for part in path.parts):
        return None
    return path.as_posix()


def relationship_source(name: str) -> str | None:
    if name == "_rels/.rels":
        return ""
    if "/_rels/" not in name or not name.endswith(".rels"):
        return None
    prefix, relation_name = name.rsplit("/_rels/", 1)
    return f"{prefix}/{relation_name[:-5]}"


def resolve_target(source: str, target: str) -> str | None:
    if not target or "\\" in target:
        return None
    value = target.lstrip("/") if target.startswith("/") else posixpath.join(posixpath.dirname(source), target)
    normalized = posixpath.normpath(value)
    if normalized == ".." or normalized.startswith("../"):
        return None
    return normalized


def chart_issues(name: str, root: ElementTree.Element) -> list[str]:
    issues: list[str] = []
    declared = {
        node.attrib.get("val")
        for axis_tag in ("catAx", "valAx", "dateAx", "serAx")
        for axis in root.iter(f"{{{C_NS}}}{axis_tag}")
        for node in axis.findall(f"{{{C_NS}}}axId")
        if node.attrib.get("val")
    }
    axis_chart_tags = (
        "areaChart", "area3DChart", "barChart", "bar3DChart", "bubbleChart",
        "lineChart", "line3DChart", "radarChart", "scatterChart", "stockChart",
        "surfaceChart", "surface3DChart",
    )
    for chart_tag in axis_chart_tags:
        for chart in root.iter(f"{{{C_NS}}}{chart_tag}"):
            references = [node.attrib.get("val") for node in chart.findall(f"{{{C_NS}}}axId") if node.attrib.get("val")]
            if len(references) < 2:
                issues.append(f"{name}: {chart_tag} 缺少完整坐标轴引用")
            missing = sorted(set(references) - declared)
            if missing:
                issues.append(f"{name}: {chart_tag} 引用未声明坐标轴 {', '.join(missing)}")
    for chart_tag in ("barChart", "bar3DChart"):
        for chart in root.iter(f"{{{C_NS}}}{chart_tag}"):
            grouping = chart.find(f"{{{C_NS}}}grouping")
            if grouping is None or grouping.attrib.get("val") not in ("stacked", "percentStacked"):
                continue
            for position in chart.iter(f"{{{C_NS}}}dLblPos"):
                if position.attrib.get("val") == "outEnd":
                    issues.append(f"{name}: 堆积图使用 outEnd 数据标签位置，PowerPoint 可能拒绝该图表")
    return issues


def inspect(path: Path) -> list[str]:
    issues: list[str] = []
    try:
        package = zipfile.ZipFile(path)
    except (OSError, zipfile.BadZipFile) as error:
        return [f"不是有效的 OOXML ZIP: {error}"]
    with package:
        names: set[str] = set()
        for info in package.infolist():
            name = canonical(info.filename)
            if name is None:
                issues.append(f"非法部件路径: {info.filename!r}")
                continue
            if name in names:
                issues.append(f"重复部件: {name}")
            names.add(name)
            mode = info.external_attr >> 16
            kind = stat.S_IFMT(mode) if mode else 0
            if kind not in (0, stat.S_IFREG, stat.S_IFDIR):
                issues.append(f"链接或特殊部件: {name}")
        issues.extend(f"缺少必需部件: {name}" for name in sorted(REQUIRED - names))
        roots: dict[str, ElementTree.Element] = {}
        for name in sorted(n for n in names if n.endswith((".xml", ".rels"))):
            try:
                roots[name] = ElementTree.fromstring(package.read(name))
            except (KeyError, ElementTree.ParseError, UnicodeError) as error:
                issues.append(f"XML 无法解析 {name}: {error}")
        for name, root in roots.items():
            if not name.endswith(".rels"):
                continue
            source = relationship_source(name)
            if source is None:
                issues.append(f"关系部件位置非法: {name}")
                continue
            seen_ids: set[str] = set()
            for relation in root.findall(f"{{{REL_NS}}}Relationship"):
                relation_id = relation.attrib.get("Id", "")
                if not relation_id or relation_id in seen_ids:
                    issues.append(f"关系 ID 缺失或重复: {name}#{relation_id or '<empty>'}")
                seen_ids.add(relation_id)
                if relation.attrib.get("TargetMode", "").lower() == "external":
                    continue
                target = resolve_target(source, relation.attrib.get("Target", ""))
                if target is None:
                    issues.append(f"关系目标非法: {name}#{relation_id}")
                elif target not in names:
                    issues.append(f"关系目标不存在: {name}#{relation_id} -> {target}")
        presentation = roots.get("ppt/presentation.xml")
        presentation_rels = roots.get("ppt/_rels/presentation.xml.rels")
        registered: set[str] = set()
        if presentation is not None and presentation_rels is not None:
            relation_targets = {
                relation.attrib.get("Id", ""): resolve_target("ppt/presentation.xml", relation.attrib.get("Target", ""))
                for relation in presentation_rels.findall(f"{{{REL_NS}}}Relationship")
                if relation.attrib.get("Type", "").endswith("/slide")
            }
            seen_slide_ids: set[str] = set()
            for slide in presentation.iter(f"{{{P_NS}}}sldId"):
                numeric_id = slide.attrib.get("id", "")
                relation_id = slide.attrib.get(f"{{{R_NS}}}id", "")
                if not numeric_id or numeric_id in seen_slide_ids:
                    issues.append(f"presentation slide id 缺失或重复: {numeric_id or '<empty>'}")
                seen_slide_ids.add(numeric_id)
                target = relation_targets.get(relation_id)
                if not target:
                    issues.append(f"slide 注册引用不存在: {relation_id or '<empty>'}")
                else:
                    registered.add(target)
            slide_parts = {name for name in names if re.fullmatch(r"ppt/slides/slide\d+\.xml", name)}
            for orphan in sorted(slide_parts - registered):
                issues.append(f"未在 presentation 注册的孤立 slide: {orphan}")
        content_types = roots.get("[Content_Types].xml")
        if content_types is not None:
            overrides = {node.attrib.get("PartName", "").lstrip("/") for node in content_types.findall(f"{{{CT_NS}}}Override")}
            for slide in sorted(registered):
                if slide not in overrides:
                    issues.append(f"slide 缺少 ContentType Override: {slide}")
        for name, root in roots.items():
            if re.fullmatch(r"ppt/slides/slide\d+\.xml", name):
                ids: list[str] = []
                for node in root.iter(f"{{{P_NS}}}cNvPr"):
                    if node.attrib.get("id"):
                        ids.append(node.attrib["id"])
                duplicates = sorted({value for value in ids if ids.count(value) > 1})
                if duplicates:
                    issues.append(f"{name}: shape id 重复 {', '.join(duplicates)}")
            if re.fullmatch(r"ppt/charts/chart\d+\.xml", name):
                issues.extend(chart_issues(name, root))
    return sorted(set(issues))


def main() -> int:
    parser = argparse.ArgumentParser(description="校验 PPTX 包结构")
    parser.add_argument("path", type=Path)
    parser.add_argument("--original", type=Path)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    issues = inspect(args.path.resolve())
    baseline = set(inspect(args.original.resolve())) if args.original else set()
    introduced = [issue for issue in issues if issue not in baseline]
    result = {
        "status": "success" if not introduced else "failed",
        "file": str(args.path.resolve()),
        "issues": introduced,
        "baseline_suppressed": len(issues) - len(introduced),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2) if args.json else ("校验通过" if not introduced else "\n".join(introduced)))
    return 0 if not introduced else 1


if __name__ == "__main__":
    raise SystemExit(main())
