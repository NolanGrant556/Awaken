#!/usr/bin/env python3
"""为 Awaken Skill 解析宿主提供的 Typst CLI。"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path


class RuntimeErrorWithCode(RuntimeError):
    """带稳定退出码的运行时错误。"""

    def __init__(self, message: str, exit_code: int = 1) -> None:
        super().__init__(message)
        self.exit_code = exit_code


def normalized_platform(value: str | None = None) -> str:
    raw = (value or sys.platform).lower()
    if raw.startswith("darwin"):
        return "darwin"
    if raw.startswith("win"):
        return "windows"
    raise RuntimeErrorWithCode(f"当前版本不支持此操作系统：{raw}")


def awaken_home() -> Path:
    raw = os.environ.get("AWAKEN_HOME", "").strip()
    if not raw:
        raise RuntimeErrorWithCode("缺少 AWAKEN_HOME；拒绝猜测或写入其他用户目录")
    root = Path(raw).expanduser()
    if not root.is_absolute():
        raise RuntimeErrorWithCode("AWAKEN_HOME 必须是绝对路径")
    return root.resolve(strict=False)


def managed_binary(root: Path | None = None, platform_name: str | None = None) -> Path:
    base = root or awaken_home()
    filename = "typst.exe" if normalized_platform(platform_name) == "windows" else "typst"
    return base / "toolchains" / "typst" / "bin" / filename


def probe_version(binary: Path | str) -> str | None:
    try:
        completed = subprocess.run(
            [str(binary), "--version"],
            check=True,
            capture_output=True,
            text=True,
            timeout=15,
        )
    except (OSError, subprocess.SubprocessError):
        return None
    output = (completed.stdout or completed.stderr).strip()
    return output.splitlines()[0] if output else None


def resolve_runtime() -> dict[str, object]:
    binding = os.environ.get("AWAKEN_TYPST")
    if binding:
        version = probe_version(binding)
        return {"path": str(Path(binding).resolve()) if version else None, "version": version,
                "source": "awaken-environment" if version else "missing", "installed": False}
    if os.environ.get("AWAKEN_CWD_RESULT") or os.environ.get("AWAKEN_ENVIRONMENT_REF") or os.environ.get("AWAKEN_OFFICE_BUNDLE_ROOT"):
        return {"path": None, "version": None, "source": "needs_runtime", "installed": False}
    fixed = managed_binary()
    fixed_version = probe_version(fixed) if fixed.is_file() else None
    if fixed_version:
        return {
            "path": str(fixed.resolve()),
            "version": fixed_version,
            "source": "awaken-managed",
            "installed": False,
        }

    system = shutil.which("typst")
    system_version = probe_version(system) if system else None
    if system and system_version:
        return {
            "path": str(Path(system).resolve()),
            "version": system_version,
            "source": "system-path",
            "installed": False,
        }

    return {
        "path": None,
        "version": None,
        "source": "missing",
        "installed": False,
    }


def install_runtime(force: bool = False) -> dict[str, object]:
    existing = resolve_runtime()
    if existing["path"] and not force:
        return existing
    raise RuntimeErrorWithCode("请调用 load_workspace_dependencies 获取固定办公包；Typst helper 不再自行安装或覆盖共享组件")


def emit(payload: dict[str, object], as_json: bool) -> None:
    if as_json:
        print(json.dumps(payload, ensure_ascii=False, sort_keys=True))
        return
    if payload.get("path"):
        print(payload["path"])
        print(f"版本：{payload['version']}")
        print(f"来源：{payload['source']}")
    else:
        print("未找到可用的 Typst CLI")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="解析 Awaken 使用的 Typst CLI；安装由共享环境管理器负责")
    subparsers = parser.add_subparsers(dest="command", required=True)
    resolve_parser = subparsers.add_parser("resolve", help="检查固定路径与系统 PATH")
    resolve_parser.add_argument("--json", action="store_true")
    install_parser = subparsers.add_parser("install", help="兼容旧调用，缺失时提示使用共享环境入口")
    install_parser.add_argument("--force", action="store_true", help="兼容旧参数；不会覆盖或安装运行环境")
    install_parser.add_argument("--json", action="store_true")
    return parser


def main() -> int:
    args = build_parser().parse_args()
    try:
        payload = resolve_runtime() if args.command == "resolve" else install_runtime(args.force)
        emit(payload, args.json)
        return 0 if payload.get("path") else 2
    except RuntimeErrorWithCode as exc:
        print(f"错误：{exc}", file=sys.stderr)
        return exc.exit_code


if __name__ == "__main__":
    raise SystemExit(main())
