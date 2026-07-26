#!/usr/bin/env python3
"""探测 PPT Skill 依赖，只报告，不自动安装。"""

from __future__ import annotations

import argparse
import importlib.util
import json
import shutil
import subprocess
from pathlib import Path


def module_available(name: str) -> bool:
    return importlib.util.find_spec(name) is not None


def command_path(*names: str) -> str | None:
    for name in names:
        found = shutil.which(name)
        if found:
            return str(Path(found).resolve())
    return None


def node_module_available(name: str) -> bool:
    node = shutil.which("node")
    if not node:
        return False
    return subprocess.run(
        [node, "-e", f"require.resolve({name!r})"],
        capture_output=True,
        text=True,
        timeout=10,
        check=False,
    ).returncode == 0


def inspect() -> dict:
    capabilities = {
        "python": command_path("python3", "python", "py"),
        "node": command_path("node"),
        "pptxgenjs": node_module_available("pptxgenjs"),
        "python_pptx": module_available("pptx"),
        "markitdown": module_available("markitdown"),
        "pillow": module_available("PIL"),
        "libreoffice": command_path("soffice", "libreoffice"),
        "pdftoppm": command_path("pdftoppm"),
    }
    missing = [name for name, value in capabilities.items() if not value]
    return {
        "status": "ready" if not missing else "missing_dependencies",
        "capabilities": capabilities,
        "missing": missing,
        "install_hints": {
            "pptxgenjs": "在任务工作区执行 npm install pptxgenjs",
            "python_pptx": "在工作区 venv 中安装 python-pptx",
            "markitdown": "在工作区 venv 中安装 markitdown[pptx]",
            "pillow": "在工作区 venv 中安装 Pillow",
            "libreoffice": "使用 Awaken 可用安装计划或系统包管理器安装 LibreOffice",
            "pdftoppm": "使用 Awaken 可用安装计划或系统包管理器安装 Poppler",
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="探测 Awaken PPT Skill 依赖")
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
