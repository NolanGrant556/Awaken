#!/usr/bin/env python3
"""Office 共用运行辅助代码；三份随 Skill 独立分发，测试保证内容一致。"""

from __future__ import annotations

import os
import platform
import shutil
import subprocess
import tempfile
from collections.abc import Sequence
from contextlib import contextmanager
from pathlib import Path


def find_office_command(name: str) -> Path | None:
    key = "AWAKEN_LIBREOFFICE" if name == "soffice" else "AWAKEN_" + name.upper()
    explicit = os.environ.get(key) or (os.environ.get("AWAKEN_SOFFICE") if name == "soffice" else None)
    root = os.environ.get("AWAKEN_OFFICE_BUNDLE_ROOT")
    if explicit:
        candidate = Path(explicit).resolve()
        if not candidate.is_file():
            return None
        if root:
            try:
                candidate.relative_to(Path(root).resolve())
            except ValueError:
                return None
        return candidate
    # Within Awaken, a missing/incomplete binding cannot fall through to an
    # unrelated system installation. Outside Awaken, retain explicit CLI use.
    if root or os.environ.get("AWAKEN_ENVIRONMENT_REF") or os.environ.get("AWAKEN_CWD_RESULT"):
        return None
    for alias in (("soffice", "libreoffice") if name == "soffice" else (name,)):
        found = shutil.which(alias)
        if found:
            return Path(found).resolve()
    if name == "soffice" and platform.system() == "Darwin":
        candidate = Path("/Applications/LibreOffice.app/Contents/MacOS/soffice")
        return candidate if candidate.is_file() else None
    if name == "soffice" and platform.system() == "Windows":
        for variable in ("PROGRAMFILES", "PROGRAMFILES(X86)"):
            base = os.environ.get(variable)
            if not base:
                continue
            candidate = Path(base) / "LibreOffice/program/soffice.exe"
            if candidate.is_file():
                return candidate
    return None


def find_soffice() -> Path | None:
    return find_office_command("soffice")


def office_env() -> dict[str, str]:
    env = os.environ.copy()
    env.setdefault("SAL_USE_VCLPLUGIN", "svp")
    return env


@contextmanager
def temporary_directory(prefix: str):
    """创建兼容 Python 3.9 的临时目录，并忽略退出阶段的清理竞争。"""
    directory = Path(tempfile.mkdtemp(prefix=prefix))
    try:
        yield directory
    finally:
        shutil.rmtree(directory, ignore_errors=True)


@contextmanager
def temporary_profile(prefix: str = "awaken-lo-profile-"):
    with temporary_directory(prefix) as directory:
        yield directory


def run_soffice(
    arguments: Sequence[str],
    *,
    timeout: int,
    profile: Path | None = None,
) -> subprocess.CompletedProcess[str]:
    executable = find_soffice()
    if executable is None:
        raise FileNotFoundError("needs_runtime: 未取得 LibreOffice；请调用 load_workspace_dependencies 获取完整办公环境")
    if profile is None:
        with temporary_profile() as generated:
            return run_soffice(arguments, timeout=timeout, profile=generated)
    profile.mkdir(parents=True, exist_ok=True)
    command = [
        str(executable),
        "--headless",
        "--nologo",
        "--norestore",
        f"-env:UserInstallation={profile.resolve().as_uri()}",
        *arguments,
    ]
    return subprocess.run(
        command,
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
        env=office_env(),
    )
