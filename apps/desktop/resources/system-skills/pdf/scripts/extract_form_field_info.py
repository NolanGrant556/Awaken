#!/usr/bin/env python3
"""列出 PDF 可填表单字段（JSON）。"""

from __future__ import annotations

import json
import sys

from pypdf import PdfReader


def make_field_dict(field_id: str, field) -> dict:
    info = {"field_id": field_id}
    ft = field.get("/FT")
    if ft == "/Tx":
        info["type"] = "text"
    elif ft == "/Btn":
        info["type"] = "checkbox"
    elif ft == "/Ch":
        info["type"] = "choice"
    else:
        info["type"] = f"unknown({ft})"
    if field.get("/V") is not None:
        info["current_value"] = str(field.get("/V"))
    return info


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: extract_form_field_info.py <form.pdf>", file=sys.stderr)
        return 2

    reader = PdfReader(sys.argv[1])
    fields = reader.get_fields() or {}
    out = []
    for field_id, field in fields.items():
        if field is None:
            continue
        if field.get("/Kids") and field.get("/FT") != "/Tx":
            continue
        item = make_field_dict(field_id, field)
        # 粗略页码：pypdf 不总提供；填表脚本里用户可再核对
        item["page"] = 1
        out.append(item)

    print(json.dumps(out, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
