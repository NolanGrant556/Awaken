#!/usr/bin/env python3
"""Fill located PDF fields from [{field_id, page, value}] into a new PDF."""
from __future__ import annotations

import io
import json
import sys
from pathlib import Path

from pypdf import PdfReader, PdfWriter

from form_fields import field_locations


def main() -> int:
    if len(sys.argv) != 4:
        print("usage: fill_fillable_fields.py <input.pdf> <values.json> <output.pdf>", file=sys.stderr)
        return 2
    input_pdf, values_path, output_pdf = map(Path, sys.argv[1:])
    if output_pdf.exists():
        raise ValueError("output already exists; choose a new path")
    values = json.loads(values_path.read_text(encoding="utf-8"))
    if not isinstance(values, list):
        raise ValueError("values must be an array of field records")
    reader = PdfReader(input_pdf)
    locations = field_locations(reader)
    selected = {}
    for item in values:
        if not isinstance(item, dict):
            raise ValueError("each field record must be an object")
        if "value" not in item:
            continue
        field_id, page, value = item.get("field_id"), item.get("page"), item["value"]
        if not isinstance(field_id, str) or field_id not in locations:
            raise ValueError(f"unknown field {field_id!r}")
        field, pages = locations[field_id]
        if type(page) is not int or page not in pages:
            raise ValueError(f"field {field_id!r} is not on page {page!r}; actual pages: {pages}")
        if not isinstance(value, str):
            raise ValueError(f"field {field_id!r} value must be a string")
        if field.get("/FT") not in {"/Tx", "/Btn", "/Ch"}:
            raise ValueError(f"unsupported field type for {field_id!r}")
        if field_id in selected and selected[field_id] != value:
            raise ValueError(f"conflicting values for repeated field {field_id!r}")
        selected[field_id] = value
    if not selected:
        raise ValueError("no field values supplied")

    by_page = {}
    for field_id, value in selected.items():
        # A shared field has one value: refresh every widget's appearance.
        for page in locations[field_id][1]:
            by_page.setdefault(page, {})[field_id] = value
    writer = PdfWriter(clone_from=reader)
    for page, field_values in by_page.items():
        writer.update_page_form_field_values(writer.pages[page - 1], field_values, auto_regenerate=False)
    buffer = io.BytesIO()
    writer.write(buffer)
    buffer.seek(0)
    actual = PdfReader(buffer).get_fields() or {}
    for field_id, value in selected.items():
        if field_id not in actual or str(actual[field_id].get("/V", "")) != value:
            raise ValueError(f"field value readback failed: {field_id!r}")
    # Exclusive creation preserves both source files and concurrently created outputs.
    with output_pdf.open("xb") as output:
        output.write(buffer.getvalue())
    print(json.dumps({"status": "success", "output": str(output_pdf),
                      "fields": sorted(selected), "pages": sorted(by_page)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        raise SystemExit(1)
