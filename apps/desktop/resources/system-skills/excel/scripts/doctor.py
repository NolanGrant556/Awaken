#!/usr/bin/env python3
"""探测 Excel Skill 的真实依赖，只报告，不自动安装。"""

from __future__ import annotations

import argparse
import importlib.util
import json
import shutil
from pathlib import Path


def command_path(*names: str) -> str | None:
    for name in names:
        found = shutil.which(name)
        if found:
            return str(Path(found).resolve())
    return None


def inspect() -> dict:
    capabilities = {
        "python": command_path("python3", "python", "py"),
        "openpyxl": importlib.util.find_spec("openpyxl") is not None,
        "pandas": importlib.util.find_spec("pandas") is not None,
        "markitdown": importlib.util.find_spec("markitdown") is not None,
        "libreoffice": command_path("soffice", "libreoffice"),
    }
    missing = [name for name, value in capabilities.items() if not value]
    return {
        "status": "ready" if not missing else "missing_dependencies",
        "capabilities": capabilities,
        "missing": missing,
        "install_hints": {
            "openpyxl": "在工作区 venv 中安装 openpyxl",
            "pandas": "在工作区 venv 中安装 pandas",
            "markitdown": "在工作区 venv 中安装 markitdown[xlsx]",
            "libreoffice": "使用 Awaken 可用安装计划或系统包管理器安装 LibreOffice",
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="探测 Awaken Excel Skill 依赖")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    result = inspect()
    if args.json:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        for name, value in result["capabilities"].items():
            print(f"{name}: {value or 'missing'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
