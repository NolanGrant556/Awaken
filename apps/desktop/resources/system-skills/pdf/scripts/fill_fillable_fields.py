#!/usr/bin/env python3
"""按 JSON 填充 PDF 可填字段。

JSON 格式示例：
[
  {"field_id": "Name", "page": 1, "value": "张三"},
  {"field_id": "Agree", "page": 1, "value": "/Yes"}
]
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

from pypdf import PdfReader, PdfWriter


def main() -> int:
    if len(sys.argv) < 4:
        print(
            "usage: fill_fillable_fields.py <input.pdf> <values.json> <output.pdf>",
            file=sys.stderr,
        )
        return 2

    input_pdf, values_path, output_pdf = sys.argv[1], sys.argv[2], sys.argv[3]
    values = json.loads(Path(values_path).read_text(encoding="utf-8"))

    by_page: dict[int, dict[str, str]] = {}
    for item in values:
        if "value" not in item:
            continue
        page = int(item.get("page", 1))
        by_page.setdefault(page, {})[item["field_id"]] = item["value"]

    reader = PdfReader(input_pdf)
    writer = PdfWriter(clone_from=reader)
    for page, field_values in by_page.items():
        idx = page - 1
        if idx < 0 or idx >= len(writer.pages):
            print(f"invalid page {page}", file=sys.stderr)
            return 1
        writer.update_page_form_field_values(writer.pages[idx], field_values, auto_regenerate=False)

    try:
        writer.set_need_appearances_writer(True)
    except Exception:
        pass

    with open(output_pdf, "wb") as f:
        writer.write(f)
    print(f"wrote {output_pdf}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
