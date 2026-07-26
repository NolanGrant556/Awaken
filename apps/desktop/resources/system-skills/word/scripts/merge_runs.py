#!/usr/bin/env python3
"""合并格式相同且仅含普通文字的相邻 Word run。"""

from __future__ import annotations

import argparse
import json
import sys
import tempfile
from pathlib import Path

sys.dont_write_bytecode = True

from lxml import etree

from ooxml_package import pack_directory, safe_extract


WORD_NS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
XML_NS = "http://www.w3.org/XML/1998/namespace"
NS = {"w": WORD_NS}
R = f"{{{WORD_NS}}}r"
RPR = f"{{{WORD_NS}}}rPr"
TEXT = f"{{{WORD_NS}}}t"


def mergeable(run: etree._Element) -> bool:
    return all(child.tag in (RPR, TEXT) for child in run)


def style_signature(run: etree._Element) -> bytes:
    properties = run.find("w:rPr", namespaces=NS)
    return b"" if properties is None else etree.tostring(properties, method="c14n")


def merge_document(document: Path) -> int:
    parser = etree.XMLParser(remove_blank_text=False, resolve_entities=False, no_network=True)
    tree = etree.parse(str(document), parser)
    merged = 0
    for paragraph in tree.xpath("//w:p", namespaces=NS):
        index = 0
        while index + 1 < len(paragraph):
            left, right = paragraph[index], paragraph[index + 1]
            if (
                left.tag != R
                or right.tag != R
                or not mergeable(left)
                or not mergeable(right)
                or style_signature(left) != style_signature(right)
            ):
                index += 1
                continue
            left_text = left.findall("w:t", namespaces=NS)
            right_text = right.findall("w:t", namespaces=NS)
            if not left_text or not right_text:
                index += 1
                continue
            combined = "".join((node.text or "") for node in left_text + right_text)
            left_text[0].text = combined
            for extra in left_text[1:]:
                left.remove(extra)
            if combined[:1].isspace() or combined[-1:].isspace():
                left_text[0].set(f"{{{XML_NS}}}space", "preserve")
            else:
                left_text[0].attrib.pop(f"{{{XML_NS}}}space", None)
            paragraph.remove(right)
            merged += 1
    if merged:
        tree.write(
            str(document),
            encoding="UTF-8",
            xml_declaration=True,
            standalone=None,
            pretty_print=False,
        )
    return merged


def process(input_path: Path, output_path: Path | None) -> tuple[int, Path | None]:
    if input_path.is_dir():
        if output_path is not None:
            raise ValueError("目录模式会原地修改，不接受 --output")
        document = input_path / "word" / "document.xml"
        if not document.is_file():
            raise ValueError("目录缺少 word/document.xml")
        return merge_document(document), None
    if output_path is None:
        raise ValueError("DOCX 文件模式必须提供 --output，避免覆盖原件")
    with tempfile.TemporaryDirectory(prefix="awaken-docx-merge-") as directory:
        unpacked = Path(directory) / "package"
        safe_extract(input_path, unpacked)
        merged = merge_document(unpacked / "word" / "document.xml")
        pack_directory(unpacked, output_path)
        return merged, output_path


def main() -> int:
    parser = argparse.ArgumentParser(description="合并 Word 普通文字 run")
    parser.add_argument("input", type=Path)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        merged, output = process(args.input.resolve(), args.output.resolve() if args.output else None)
        print(json.dumps({"status": "success", "merged_runs": merged, "output": str(output) if output else None}, ensure_ascii=False))
        return 0
    except Exception as error:
        print(json.dumps({"status": "failed", "error": str(error)}, ensure_ascii=False))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
