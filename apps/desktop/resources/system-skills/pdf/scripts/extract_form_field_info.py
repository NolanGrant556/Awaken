#!/usr/bin/env python3
"""列出 PDF 可填表单字段（JSON）。"""

from __future__ import annotations

import json
import sys

from pypdf import PdfReader

from form_fields import field_locations


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
    out = []
    for field_id, (field, pages) in field_locations(reader).items():
        item = make_field_dict(field_id, field)
        if not pages:
            out.append({**item, "page": None, "warning": "no visible widget; cannot fill by page"})
        else:
            # One row per real widget page preserves the existing page contract.
            out.extend({**item, "page": page} for page in pages)

    print(json.dumps(out, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        raise SystemExit(1)
