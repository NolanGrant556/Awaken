#!/usr/bin/env python3
"""按 PPT 工作路线探测真实能力，只报告，不自动安装。"""

from __future__ import annotations

import argparse
import importlib.util
import json
import os
import platform
import subprocess
import sys
from pathlib import Path

from office_runtime import find_office_command


ROUTE_NAMES = ("read", "create", "edit", "structural_qa", "template_map", "visual_qa")
MINIMUM_PYTHON = (3, 9)


def module_available(name: str) -> bool:
    return importlib.util.find_spec(name) is not None


def command_path(*names: str) -> str | None:
    for name in names:
        found = find_office_command(name)
        if found:
            return str(found)
    return None


def libreoffice_path() -> str | None:
    return command_path("soffice")


def node_module_available(name: str, dependency_root: Path | None = None) -> bool:
    node = command_path("node")
    if not node:
        return False
    root = Path(os.environ["AWAKEN_NODE_PACKAGE_ROOT"]).resolve() if os.environ.get("AWAKEN_NODE_PACKAGE_ROOT") else (dependency_root or Path.cwd()).resolve()
    try:
        result = subprocess.run(
            [
                node,
                "-e",
                "require.resolve(process.argv[1], { paths: [process.argv[2]] })",
                name,
                str(root),
            ],
            capture_output=True,
            text=True,
            timeout=10,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        return False
    return result.returncode == 0


def capability(available: bool, path: str | Path | None = None, **details) -> dict:
    result = {"available": bool(available)}
    if path:
        result["path"] = str(Path(path).absolute())
    result.update(details)
    return result


def required_route(capabilities: dict, required: list[str]) -> dict:
    missing = [name for name in required if not capabilities[name]["available"]]
    return {
        "status": "ready" if not missing else "unavailable",
        "required": required,
        "missing": missing,
    }


def alternative_route(capabilities: dict, alternatives: list[list[str]]) -> dict:
    ready = [
        alternative
        for alternative in alternatives
        if all(capabilities[name]["available"] for name in alternative)
    ]
    return {
        "status": "ready" if ready else "unavailable",
        "any_of": alternatives,
        "ready_alternatives": ready,
        "missing_alternatives": [
            {
                "required": alternative,
                "missing": [name for name in alternative if not capabilities[name]["available"]],
            }
            for alternative in alternatives
            if alternative not in ready
        ],
    }


def inspect(route: str | None = None, dependency_root: Path | None = None) -> dict:
    script_dir = Path(__file__).resolve().parent
    dependency_root = Path(os.environ["AWAKEN_NODE_PACKAGE_ROOT"]).resolve() if os.environ.get("AWAKEN_NODE_PACKAGE_ROOT") else (dependency_root or Path.cwd()).resolve()
    python_compatible = sys.version_info[:2] >= MINIMUM_PYTHON
    node_path = command_path("node")
    office_path = libreoffice_path()
    pdftoppm_path = command_path("pdftoppm")
    bundled_paths = {
        "bundled_renderer": script_dir / "render_slides.py",
        "bundled_thumbnailer": script_dir / "thumbnail.py",
        "bundled_validator": script_dir / "validate_pptx.py",
        "bundled_ooxml_editor": script_dir / "ooxml_package.py",
    }
    capabilities = {
        "python": capability(
            python_compatible,
            sys.executable,
            version=platform.python_version(),
            minimum_version="3.9",
        ),
        "node": capability(bool(node_path), node_path),
        "pptxgenjs": capability(node_module_available("pptxgenjs", dependency_root)),
        "python_pptx": capability(module_available("pptx")),
        "markitdown": capability(module_available("markitdown")),
        "pillow": capability(module_available("PIL")),
        "libreoffice": capability(bool(office_path), office_path),
        "pdftoppm": capability(bool(pdftoppm_path), pdftoppm_path),
        **{
            name: capability(path.is_file(), path)
            for name, path in bundled_paths.items()
        },
    }
    routes = {
        "read": required_route(capabilities, ["python", "markitdown"]),
        "create": alternative_route(
            capabilities,
            [["node", "pptxgenjs"], ["python", "python_pptx"]],
        ),
        "edit": alternative_route(
            capabilities,
            [
                ["python", "python_pptx"],
                ["python", "bundled_ooxml_editor", "bundled_validator"],
            ],
        ),
        "structural_qa": required_route(capabilities, ["python", "bundled_validator"]),
        "template_map": required_route(
            capabilities,
            [
                "python",
                "pillow",
                "libreoffice",
                "pdftoppm",
                "bundled_thumbnailer",
                "bundled_renderer",
            ],
        ),
        "visual_qa": required_route(
            capabilities,
            ["python", "libreoffice", "pdftoppm", "bundled_renderer"],
        ),
    }
    if route:
        routes = {route: routes[route]}
        referenced = set(routes[route].get("required", []))
        for alternative in routes[route].get("any_of", []):
            referenced.update(alternative)
        capabilities = {name: value for name, value in capabilities.items() if name in referenced}
    ready_routes = [name for name, value in routes.items() if value["status"] == "ready"]
    unavailable_routes = [name for name, value in routes.items() if value["status"] != "ready"]
    blocking_missing: set[str] = set()
    for route_result in routes.values():
        if route_result["status"] == "ready":
            continue
        blocking_missing.update(route_result.get("missing", []))
        for alternative in route_result.get("missing_alternatives", []):
            blocking_missing.update(alternative["missing"])
    all_hints = {
        "python": "使用 Python 3.9 或更高版本执行 PPT Skill",
        "node": "调用 load_workspace_dependencies，输入 {}",
        "pptxgenjs": "调用 load_workspace_dependencies，输入 {}",
        "python_pptx": "调用 load_workspace_dependencies，输入 {}",
        "markitdown": "调用 load_workspace_dependencies，输入 {}",
        "pillow": "调用 load_workspace_dependencies，输入 {}",
        "libreoffice": "调用 load_workspace_dependencies，输入 {}",
        "pdftoppm": "调用 load_workspace_dependencies，输入 {}",
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
        "environment_ref": os.environ.get("AWAKEN_ENVIRONMENT_REF"),
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
    parser = argparse.ArgumentParser(description="按工作路线探测 Awaken PPT Skill 能力")
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
