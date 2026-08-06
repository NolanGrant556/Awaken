#!/usr/bin/env python3
"""按 Excel 工作路线探测真实能力，只报告，不自动安装。"""

from __future__ import annotations

import argparse
import importlib.util
import json
import os
import platform
import shutil
import sys
from pathlib import Path


ROUTE_NAMES = ("read_precise", "read_quick", "clean", "create_edit", "recalc")
MINIMUM_PYTHON = (3, 9)


def command_path(*names: str) -> str | None:
    for name in names:
        found = shutil.which(name)
        if found:
            return str(Path(found).resolve())
    return None


def libreoffice_path() -> str | None:
    found = command_path("soffice", "libreoffice")
    if found:
        return found
    if platform.system() == "Darwin":
        candidate = Path("/Applications/LibreOffice.app/Contents/MacOS/soffice")
        return str(candidate) if candidate.is_file() else None
    if platform.system() == "Windows":
        for env_name in ("PROGRAMFILES", "PROGRAMFILES(X86)"):
            base = os.environ.get(env_name)
            if not base:
                continue
            candidate = Path(base) / "LibreOffice" / "program" / "soffice.exe"
            if candidate.is_file():
                return str(candidate)
    return None


def capability(available: bool, path: str | Path | None = None, **details) -> dict:
    result = {"available": bool(available)}
    if path:
        result["path"] = str(Path(path).resolve())
    result.update(details)
    return result


def required_route(capabilities: dict, required: list[str]) -> dict:
    missing = [name for name in required if not capabilities[name]["available"]]
    return {
        "status": "ready" if not missing else "unavailable",
        "required": required,
        "missing": missing,
    }


def inspect(route: str | None = None, dependency_root: Path | None = None) -> dict:
    script_dir = Path(__file__).resolve().parent
    dependency_root = (dependency_root or Path.cwd()).resolve()
    python_compatible = sys.version_info[:2] >= MINIMUM_PYTHON
    office_path = libreoffice_path()
    bundled_paths = {
        "bundled_inspector": script_dir / "inspect_workbook.py",
        "bundled_recalculator": script_dir / "recalc.py",
    }
    capabilities = {
        "python": capability(
            python_compatible,
            sys.executable,
            version=platform.python_version(),
            minimum_version="3.9",
        ),
        "openpyxl": capability(importlib.util.find_spec("openpyxl") is not None),
        "pandas": capability(importlib.util.find_spec("pandas") is not None),
        "markitdown": capability(importlib.util.find_spec("markitdown") is not None),
        "libreoffice": capability(bool(office_path), office_path),
        **{
            name: capability(path.is_file(), path)
            for name, path in bundled_paths.items()
        },
    }
    routes = {
        "read_precise": required_route(capabilities, ["python", "openpyxl", "bundled_inspector"]),
        "read_quick": required_route(capabilities, ["python", "markitdown"]),
        "clean": required_route(capabilities, ["python", "pandas"]),
        "create_edit": required_route(capabilities, ["python", "openpyxl"]),
        "recalc": required_route(
            capabilities,
            ["python", "openpyxl", "libreoffice", "bundled_recalculator"],
        ),
    }
    if route:
        routes = {route: routes[route]}
        referenced = set(routes[route]["required"])
        capabilities = {name: value for name, value in capabilities.items() if name in referenced}
    ready_routes = [name for name, value in routes.items() if value["status"] == "ready"]
    unavailable_routes = [name for name, value in routes.items() if value["status"] != "ready"]
    blocking_missing = {
        name
        for route_result in routes.values()
        if route_result["status"] != "ready"
        for name in route_result["missing"]
    }
    all_hints = {
        "python": "使用 Python 3.9 或更高版本执行 Excel Skill",
        "openpyxl": "在工作区 venv 中安装 openpyxl",
        "pandas": "在工作区 venv 中安装 pandas",
        "markitdown": "在工作区 venv 中安装 markitdown[xlsx]",
        "libreoffice": "使用 Awaken 可用安装计划或系统包管理器安装 LibreOffice",
    }
    return {
        "status": "ready" if not unavailable_routes else "partial",
        "selected_route": route,
        "routes": routes,
        "capabilities": capabilities,
        "ready_routes": ready_routes,
        "unavailable_routes": unavailable_routes,
        "install_hints": {
            name: all_hints[name]
            for name in sorted(blocking_missing)
            if name in all_hints
        },
        "execution_plan": {
            "dependency_root": str(dependency_root),
            **{
                name: value["path"]
                for name, value in capabilities.items()
                if value.get("available") and value.get("path")
            },
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="按工作路线探测 Awaken Excel Skill 能力")
    parser.add_argument("--json", action="store_true")
    parser.add_argument("--route", choices=ROUTE_NAMES)
    parser.add_argument("--dependency-root", type=Path, default=Path.cwd())
    args = parser.parse_args()
    result = inspect(args.route, args.dependency_root)
    if args.json:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        for name, route in result["routes"].items():
            missing = route.get("missing", [])
            suffix = f" (missing: {', '.join(missing)})" if missing else ""
            print(f"{name}: {route['status']}{suffix}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
