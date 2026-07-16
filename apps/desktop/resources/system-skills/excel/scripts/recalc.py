#!/usr/bin/env python3
"""用 LibreOffice 重算 xlsx 公式，并扫描常见错误单元格。输出 JSON。

注意：会就地覆写目标文件（LibreOffice 往返转换），图表等 openpyxl
不支持的对象可能丢失；需要保留原件请先自行备份。
"""

from __future__ import annotations

import json
import os
import platform
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ERROR_MARKERS = ("#REF!", "#DIV/0!", "#VALUE!", "#N/A", "#NAME?", "#NULL!", "#NUM!")


def find_soffice() -> str | None:
    for name in ("soffice", "libreoffice"):
        path = shutil.which(name)
        if path:
            return path
    system = platform.system()
    if system == "Darwin":
        mac = Path("/Applications/LibreOffice.app/Contents/MacOS/soffice")
        if mac.exists():
            return str(mac)
    elif system == "Windows":
        for env_name in ("PROGRAMFILES", "PROGRAMFILES(X86)"):
            base = os.environ.get(env_name)
            if not base:
                continue
            candidate = Path(base) / "LibreOffice" / "program" / "soffice.exe"
            if candidate.exists():
                return str(candidate)
    return None


def scan_errors(path: Path) -> dict:
    try:
        from openpyxl import load_workbook
    except ImportError:
        return {"warning": "openpyxl not installed; skipped error scan"}

    wb = load_workbook(path, data_only=True)
    summary: dict[str, dict] = {}
    formula_wb = load_workbook(path, data_only=False)
    total_formulas = 0
    for sheet in formula_wb.worksheets:
        for row in sheet.iter_rows():
            for cell in row:
                if isinstance(cell.value, str) and cell.value.startswith("="):
                    total_formulas += 1

    total_errors = 0
    for sheet in wb.worksheets:
        for row in sheet.iter_rows():
            for cell in row:
                value = cell.value
                if isinstance(value, str) and value in ERROR_MARKERS:
                    total_errors += 1
                    bucket = summary.setdefault(value, {"count": 0, "locations": []})
                    bucket["count"] += 1
                    if len(bucket["locations"]) < 50:
                        bucket["locations"].append(f"{sheet.title}!{cell.coordinate}")

    status = "errors_found" if total_errors else "success"
    out = {"status": status, "total_errors": total_errors, "total_formulas": total_formulas}
    if summary:
        out["error_summary"] = summary
    return out


def main() -> int:
    if len(sys.argv) < 2:
        print(json.dumps({"error": "usage: recalc.py <file.xlsx> [timeout_seconds]"}, ensure_ascii=False))
        return 2

    src = Path(sys.argv[1]).expanduser().resolve()
    timeout = int(sys.argv[2]) if len(sys.argv) > 2 else 60
    if not src.exists():
        print(json.dumps({"error": f"file not found: {src}"}, ensure_ascii=False))
        return 1

    soffice = find_soffice()
    if not soffice:
        print(json.dumps({"error": "LibreOffice (soffice) not found on PATH"}, ensure_ascii=False))
        return 1

    with tempfile.TemporaryDirectory(prefix="awaken-xlsx-recalc-") as tmp:
        tmp_path = Path(tmp)
        work = tmp_path / src.name
        work.write_bytes(src.read_bytes())
        cmd = [
            soffice,
            "--headless",
            "--nologo",
            "--nolockcheck",
            "--convert-to",
            "xlsx",
            "--outdir",
            str(tmp_path),
            str(work),
        ]
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
        except subprocess.TimeoutExpired:
            print(json.dumps({"error": f"LibreOffice timed out after {timeout}s"}, ensure_ascii=False))
            return 1

        converted = tmp_path / src.name
        if not converted.exists():
            # LibreOffice sometimes changes casing/extension handling
            candidates = list(tmp_path.glob("*.xlsx"))
            if not candidates:
                print(
                    json.dumps(
                        {
                            "error": "conversion produced no xlsx",
                            "stdout": proc.stdout[-2000:],
                            "stderr": proc.stderr[-2000:],
                        },
                        ensure_ascii=False,
                    )
                )
                return 1
            converted = candidates[0]

        src.write_bytes(converted.read_bytes())

    result = scan_errors(src)
    result["file"] = str(src)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result.get("status") == "success" else 1


if __name__ == "__main__":
    raise SystemExit(main())
