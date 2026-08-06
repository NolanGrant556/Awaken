#!/usr/bin/env python3
"""生成带页码标签的 PPT 缩略图网格，用于模板布局分析。"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

sys.dont_write_bytecode = True

from PIL import Image, ImageDraw, ImageFont

from office_runtime import temporary_directory
from render_slides import render


def build(input_path: Path, output_path: Path, columns: int) -> dict:
    if output_path.exists():
        raise ValueError(f"输出已存在，请使用新文件名: {output_path}")
    columns = max(1, min(columns, 6))
    with temporary_directory("awaken-ppt-thumbnail-") as directory:
        rendered_dir = directory / "slides"
        result = render(input_path, rendered_dir, dpi=96, timeout=180)
        images = [Path(path) for path in result["images"]]
        if not images:
            raise RuntimeError("没有可用于缩略图的页面")
        thumb_width = 320
        label_height = 36
        gap = 20
        with Image.open(images[0]) as first:
            thumb_height = round(thumb_width * first.height / first.width)
        rows = (len(images) + columns - 1) // columns
        canvas = Image.new(
            "RGB",
            (gap + columns * (thumb_width + gap), gap + rows * (thumb_height + label_height + gap)),
            "white",
        )
        draw = ImageDraw.Draw(canvas)
        font = ImageFont.load_default()
        for index, path in enumerate(images, start=1):
            row, column = divmod(index - 1, columns)
            x = gap + column * (thumb_width + gap)
            y = gap + row * (thumb_height + label_height + gap)
            label = f"Slide {index}"
            draw.text((x, y), label, fill="black", font=font)
            with Image.open(path) as source:
                preview = source.convert("RGB")
                preview.thumbnail((thumb_width, thumb_height), Image.Resampling.LANCZOS)
                canvas.paste(preview, (x, y + label_height))
                draw.rectangle(
                    [x, y + label_height, x + preview.width - 1, y + label_height + preview.height - 1],
                    outline="#9CA3AF",
                    width=1,
                )
        output_path.parent.mkdir(parents=True, exist_ok=True)
        canvas.save(output_path, quality=92)
    return {"status": "success", "slides": len(images), "output": str(output_path.resolve())}


def main() -> int:
    parser = argparse.ArgumentParser(description="生成 PPT 缩略图网格")
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--columns", type=int, default=3)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    try:
        result = build(args.input.resolve(), args.output.resolve(), args.columns)
        print(json.dumps(result, ensure_ascii=False, indent=2) if args.json else result["output"])
        return 0
    except Exception as error:
        result = {"status": "failed", "error": str(error)}
        print(json.dumps(result, ensure_ascii=False, indent=2) if args.json else result["error"])
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
