#!/usr/bin/env python3
"""对 DOCX/DOTX 执行 Awaken 自有的包结构和关系校验。"""

from __future__ import annotations

import argparse
import json
import posixpath
import stat
import zipfile
from pathlib import Path, PurePosixPath
from xml.etree import ElementTree


REQUIRED_PARTS = {"[Content_Types].xml", "_rels/.rels", "word/document.xml"}
REL_NS = "http://schemas.openxmlformats.org/package/2006/relationships"
WORD_NS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"


def canonical_name(name: str) -> str | None:
    if not name or name.startswith("/") or "\\" in name:
        return None
    path = PurePosixPath(name.rstrip("/"))
    if any(part in ("", ".", "..") for part in path.parts):
        return None
    return path.as_posix()


def source_for_relationship_part(name: str) -> str | None:
    if name == "_rels/.rels":
        return ""
    marker = "/_rels/"
    if marker not in name or not name.endswith(".rels"):
        return None
    prefix, relation_name = name.rsplit(marker, 1)
    return f"{prefix}/{relation_name[:-5]}"


def resolve_target(source: str, target: str) -> str | None:
    if not target or "\\" in target:
        return None
    candidate = target.lstrip("/") if target.startswith("/") else posixpath.join(posixpath.dirname(source), target)
    normalized = posixpath.normpath(candidate)
    if normalized in ("", ".") or normalized == ".." or normalized.startswith("../"):
        return None
    return normalized


def inspect(path: Path) -> list[str]:
    issues: list[str] = []
    try:
        package = zipfile.ZipFile(path)
    except (OSError, zipfile.BadZipFile) as error:
        return [f"不是有效的 OOXML ZIP: {error}"]
    with package:
        names: set[str] = set()
        for info in package.infolist():
            normalized = canonical_name(info.filename)
            if normalized is None:
                issues.append(f"非法部件路径: {info.filename!r}")
                continue
            if normalized in names:
                issues.append(f"重复部件: {normalized}")
            names.add(normalized)
            mode = info.external_attr >> 16
            kind = stat.S_IFMT(mode) if mode else 0
            if kind not in (0, stat.S_IFREG, stat.S_IFDIR):
                issues.append(f"链接或特殊部件: {normalized}")
        for missing in sorted(REQUIRED_PARTS - names):
            issues.append(f"缺少必需部件: {missing}")
        xml_roots: dict[str, ElementTree.Element] = {}
        for name in sorted(n for n in names if n.endswith((".xml", ".rels"))):
            try:
                xml_roots[name] = ElementTree.fromstring(package.read(name))
            except (KeyError, ElementTree.ParseError, UnicodeError) as error:
                issues.append(f"XML 无法解析 {name}: {error}")
        for name, root in xml_roots.items():
            if not name.endswith(".rels"):
                continue
            source = source_for_relationship_part(name)
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
        document = xml_roots.get("word/document.xml")
        comments = xml_roots.get("word/comments.xml")
        if document is not None:
            references = {
                node.attrib.get(f"{{{WORD_NS}}}id")
                for node in document.iter(f"{{{WORD_NS}}}commentReference")
            }
            references.discard(None)
            if references and comments is None:
                issues.append("正文含批注引用，但缺少 word/comments.xml")
            if comments is not None:
                comment_ids = {
                    node.attrib.get(f"{{{WORD_NS}}}id")
                    for node in comments.iter(f"{{{WORD_NS}}}comment")
                }
                missing_comments = sorted(references - comment_ids)
                if missing_comments:
                    issues.append(f"批注引用没有对应批注正文: {', '.join(missing_comments)}")
        for name, root in xml_roots.items():
            if not name.startswith("word/") or not name.endswith(".xml"):
                continue
            for tag in ("ins", "del"):
                for node in root.iter(f"{{{WORD_NS}}}{tag}"):
                    for attribute in ("id", "author", "date"):
                        if not node.attrib.get(f"{{{WORD_NS}}}{attribute}"):
                            issues.append(f"{name}: 修订节点 w:{tag} 缺少 w:{attribute}")
    return sorted(set(issues))


def main() -> int:
    parser = argparse.ArgumentParser(description="校验 DOCX 包结构")
    parser.add_argument("path", type=Path)
    parser.add_argument("--original", type=Path, help="将原文件已有问题作为基线")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    issues = inspect(args.path.resolve())
    baseline = inspect(args.original.resolve()) if args.original else []
    baseline_set = set(baseline)
    introduced = [issue for issue in issues if issue not in baseline_set]
    result = {
        "status": "success" if not introduced else "failed",
        "file": str(args.path.resolve()),
        "issues": introduced,
        "baseline_suppressed": len(issues) - len(introduced),
    }
    if args.json:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        print("校验通过" if not introduced else "\n".join(introduced))
    return 0 if not introduced else 1


if __name__ == "__main__":
    raise SystemExit(main())
