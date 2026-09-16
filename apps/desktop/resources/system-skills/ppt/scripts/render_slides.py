#!/usr/bin/env python3
"""严格把 PPTX 转为逐页 JPEG，拒绝旧 PDF/旧页图假成功。"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
import zipfile
from pathlib import Path
from xml.etree import ElementTree

sys.dont_write_bytecode = True

from office_runtime import find_office_command, run_soffice, temporary_directory


P_NS = "http://schemas.openxmlformats.org/presentationml/2006/main"


def slide_counts(path: Path) -> tuple[int, int]:
    with zipfile.ZipFile(path) as package:
        presentation = ElementTree.fromstring(package.read("ppt/presentation.xml"))
        total = sum(1 for _ in presentation.iter(f"{{{P_NS}}}sldId"))
        hidden = 0
        for name in package.namelist():
            if not re.fullmatch(r"ppt/slides/slide\d+\.xml", name):
                continue
            root = ElementTree.fromstring(package.read(name))
            if root.attrib.get("show", "1").lower() in ("0", "false"):
                hidden += 1
    return total, hidden


def numeric_key(path: Path) -> tuple[int, str]:
    match = re.search(r"(\d+)$", path.stem)
    return (int(match.group(1)) if match else 0, path.name)


def render(input_path: Path, output_dir: Path, dpi: int, timeout: int) -> dict:
    if not input_path.is_file():
        raise FileNotFoundError(f"输入文件不存在: {input_path}")
    if output_dir.exists() and any(output_dir.iterdir()):
        raise ValueError(f"预览目录不是空目录，请为本轮 QA 使用新目录: {output_dir}")
    pdftoppm = find_office_command("pdftoppm")
    if not pdftoppm:
        raise FileNotFoundError("needs_runtime: 未取得 Poppler；请调用 load_workspace_dependencies")
    declared, hidden = slide_counts(input_path)
    if declared == 0:
        raise ValueError("演示文稿没有注册任何 slide")
    output_dir.mkdir(parents=True, exist_ok=True)
    with temporary_directory("awaken-ppt-render-") as temporary:
        result = run_soffice(
            ["--convert-to", "pdf", "--outdir", str(temporary), str(input_path.resolve())],
            timeout=timeout,
        )
        pdf = temporary / f"{input_path.stem}.pdf"
        if result.returncode != 0 or not pdf.is_file() or pdf.stat().st_size == 0:
            detail = (result.stderr or result.stdout or "LibreOffice 未生成目标 PDF").strip()
            raise RuntimeError(detail)
        image_result = subprocess.run(
            [pdftoppm, "-jpeg", "-r", str(dpi), str(pdf), str(temporary / "slide")],
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
        if image_result.returncode != 0:
            raise RuntimeError((image_result.stderr or image_result.stdout or "pdftoppm 失败").strip())
        temporary_images = sorted(temporary.glob("slide-*.jpg"), key=numeric_key)
        visible = declared - hidden
        if len(temporary_images) not in {declared, visible}:
            raise RuntimeError(
                f"渲染页数与 PPTX 注册不一致: declared={declared}, hidden={hidden}, rendered={len(temporary_images)}"
            )
        images: list[str] = []
        for index, source in enumerate(temporary_images, start=1):
            destination = output_dir / f"slide-{index:03d}.jpg"
            shutil.copy2(source, destination)
            images.append(str(destination.resolve()))
    return {
        "status": "success",
        "declared_slides": declared,
        "hidden_slides": hidden,
        "rendered_slides": len(images),
        "images": images,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="渲染 PPTX 供逐页视觉 QA")
    parser.add_argument("input", type=Path)
    parser.add_argument("output_dir", type=Path)
    parser.add_argument("--dpi", type=int, default=150)
    parser.add_argument("--timeout", type=int, default=180)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    try:
        result = render(args.input.resolve(), args.output_dir.resolve(), args.dpi, args.timeout)
        print(json.dumps(result, ensure_ascii=False, indent=2) if args.json else "\n".join(result["images"]))
        return 0
    except Exception as error:
        result = {"status": "failed", "error": str(error)}
        print(json.dumps(result, ensure_ascii=False, indent=2) if args.json else result["error"])
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
