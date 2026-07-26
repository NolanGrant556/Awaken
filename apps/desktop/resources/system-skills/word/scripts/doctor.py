#!/usr/bin/env python3
"""探测 Word Skill 的真实本机依赖，只报告，不自动安装。"""

from __future__ import annotations

import argparse
import importlib.util
import json
import shutil
import subprocess
from pathlib import Path


def module_available(name: str) -> bool:
    return importlib.util.find_spec(name) is not None


def node_module_available(name: str) -> bool:
    node = shutil.which("node")
    if not node:
        return False
    result = subprocess.run(
        [node, "-e", f"require.resolve({name!r})"],
        capture_output=True,
        text=True,
        timeout=10,
        check=False,
    )
    return result.returncode == 0


def command_path(*names: str) -> str | None:
    for name in names:
        found = shutil.which(name)
        if found:
            return str(Path(found).resolve())
    return None


def inspect() -> dict:
    capabilities = {
        "python": command_path("python3", "python", "py"),
        "node": command_path("node"),
        "docx_node": node_module_available("docx"),
        "python_docx": module_available("docx"),
        "lxml": module_available("lxml"),
        "markitdown": module_available("markitdown"),
        "pandoc": command_path("pandoc"),
        "libreoffice": command_path("soffice", "libreoffice"),
        "pdftoppm": command_path("pdftoppm"),
    }
    missing = [name for name, value in capabilities.items() if not value]
    return {
        "status": "ready" if not missing else "missing_dependencies",
        "capabilities": capabilities,
        "missing": missing,
        "install_hints": {
            "docx_node": "在任务工作区执行 npm install docx",
            "python_docx": "在工作区 venv 中安装 python-docx",
            "lxml": "在工作区 venv 中安装 lxml",
            "markitdown": "在工作区 venv 中安装 markitdown[docx]",
            "pandoc": "使用系统包管理器安装 Pandoc",
            "libreoffice": "使用 Awaken 可用安装计划或系统包管理器安装 LibreOffice",
            "pdftoppm": "使用 Awaken 可用安装计划或系统包管理器安装 Poppler",
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="探测 Awaken Word Skill 依赖")
    parser.add_argument("--json", action="store_true", help="输出 JSON")
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
