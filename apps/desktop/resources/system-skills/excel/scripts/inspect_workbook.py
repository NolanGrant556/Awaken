#!/usr/bin/env python3
"""输出 Excel 工作簿的公式、缓存、宏、外链和兼容风险摘要。"""

from __future__ import annotations

import argparse
import json
import re
import zipfile
from collections import Counter
from pathlib import Path

from openpyxl import load_workbook


ERROR_VALUES = {"#REF!", "#DIV/0!", "#VALUE!", "#N/A", "#NAME?", "#NULL!", "#NUM!"}
RISKY_FUNCTIONS = ("XLOOKUP", "XMATCH", "FILTER", "SORT", "UNIQUE", "SEQUENCE")


def inspect(path: Path) -> dict:
    keep_vba = path.suffix.lower() == ".xlsm"
    with zipfile.ZipFile(path) as package:
        names = set(package.namelist())
    formulas_book = load_workbook(path, data_only=False, read_only=False, keep_vba=keep_vba)
    values_book = load_workbook(path, data_only=True, read_only=False, keep_vba=keep_vba)
    formula_count = 0
    missing_cache = 0
    errors: list[str] = []
    risky: list[dict[str, str]] = []
    external_formula_cells: list[str] = []
    sheets: list[dict] = []
    for sheet_name in formulas_book.sheetnames:
        formula_sheet = formulas_book[sheet_name]
        value_sheet = values_book[sheet_name]
        sheets.append(
            {
                "name": sheet_name,
                "state": formula_sheet.sheet_state,
                "max_row": formula_sheet.max_row,
                "max_column": formula_sheet.max_column,
                "merged_ranges": len(formula_sheet.merged_cells.ranges),
            }
        )
        for row in formula_sheet.iter_rows():
            for cell in row:
                value = cell.value
                cached = value_sheet[cell.coordinate].value
                location = f"{sheet_name}!{cell.coordinate}"
                if isinstance(value, str) and value.startswith("="):
                    formula_count += 1
                    if cached is None:
                        missing_cache += 1
                    upper = value.upper()
                    for function in RISKY_FUNCTIONS:
                        if re.search(rf"(?<![A-Z0-9_.])(?:_XLFN\.)?{function}\s*\(", upper):
                            risky.append({"cell": location, "function": function})
                    if re.search(r"'?\[[^\]]+\]", value):
                        external_formula_cells.append(location)
                if isinstance(cached, str) and cached in ERROR_VALUES:
                    errors.append(f"{location}:{cached}")
    formulas_book.close()
    values_book.close()
    error_counts = Counter(item.rsplit(":", 1)[1] for item in errors)
    external_parts = sorted(name for name in names if name.startswith("xl/externalLinks/") and name.endswith(".xml"))
    return {
        "status": "success",
        "file": str(path),
        "sheets": sheets,
        "formula_count": formula_count,
        "missing_formula_cache": missing_cache,
        "formula_errors": errors[:200],
        "formula_error_counts": dict(error_counts),
        "formula_errors_truncated": max(0, len(errors) - 200),
        "has_vba": "xl/vbaProject.bin" in names,
        "external_link_parts": external_parts,
        "external_formula_cells": external_formula_cells[:200],
        "risky_formulas": risky[:200],
        "risks_truncated": max(0, len(risky) - 200),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="检查 Excel 工作簿")
    parser.add_argument("path", type=Path)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    try:
        result = inspect(args.path.resolve())
        print(json.dumps(result, ensure_ascii=False, indent=2) if args.json else json.dumps(result, ensure_ascii=False))
        return 0
    except Exception as error:
        result = {"status": "failed", "error": str(error)}
        print(json.dumps(result, ensure_ascii=False, indent=2) if args.json else result["error"])
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
