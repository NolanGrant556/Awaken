#!/usr/bin/env python3
"""严格把 DOCX 转为逐页 JPEG；拒绝复用旧预览。"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

sys.dont_write_bytecode = True

from office_runtime import run_soffice


def numeric_key(path: Path) -> tuple[int, str]:
    match = re.search(r"(\d+)$", path.stem)
    return (int(match.group(1)) if match else 0, path.name)


def render(input_path: Path, output_dir: Path, dpi: int, timeout: int) -> dict:
    if not input_path.is_file():
        raise FileNotFoundError(f"输入文件不存在: {input_path}")
    if output_dir.exists() and any(output_dir.iterdir()):
        raise ValueError(f"预览目录不是空目录，请为本轮 QA 使用新目录: {output_dir}")
    pdftoppm = shutil.which("pdftoppm")
    if not pdftoppm:
        raise FileNotFoundError("未找到 pdftoppm（Poppler）")
    output_dir.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="awaken-word-render-", ignore_cleanup_errors=True) as directory:
        temporary = Path(directory)
        result = run_soffice(
            ["--convert-to", "pdf", "--outdir", str(temporary), str(input_path.resolve())],
            timeout=timeout,
        )
        pdf = temporary / f"{input_path.stem}.pdf"
        if result.returncode != 0 or not pdf.is_file() or pdf.stat().st_size == 0:
            detail = (result.stderr or result.stdout or "LibreOffice 未生成目标 PDF").strip()
            raise RuntimeError(detail)
        prefix = temporary / "page"
        image_result = subprocess.run(
            [pdftoppm, "-jpeg", "-r", str(dpi), str(pdf), str(prefix)],
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
        if image_result.returncode != 0:
            raise RuntimeError((image_result.stderr or image_result.stdout or "pdftoppm 失败").strip())
        temporary_images = sorted(temporary.glob("page-*.jpg"), key=numeric_key)
        if not temporary_images:
            raise RuntimeError("没有生成任何页面图片")
        images: list[str] = []
        for index, source in enumerate(temporary_images, start=1):
            destination = output_dir / f"page-{index:03d}.jpg"
            shutil.copy2(source, destination)
            images.append(str(destination.resolve()))
    return {"status": "success", "pages": len(images), "images": images}


def main() -> int:
    parser = argparse.ArgumentParser(description="渲染 DOCX 供视觉 QA")
    parser.add_argument("input", type=Path)
    parser.add_argument("output_dir", type=Path)
    parser.add_argument("--dpi", type=int, default=120)
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
