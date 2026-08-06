#!/usr/bin/env python3
"""Awaken PPT Skill 的 LibreOffice 进程辅助函数。"""

from __future__ import annotations

import os
import platform
import shutil
import subprocess
import tempfile
from collections.abc import Sequence
from contextlib import contextmanager
from pathlib import Path


def find_soffice() -> Path | None:
    for name in ("soffice", "libreoffice"):
        found = shutil.which(name)
        if found:
            return Path(found).resolve()
    if platform.system() == "Darwin":
        candidate = Path("/Applications/LibreOffice.app/Contents/MacOS/soffice")
        return candidate if candidate.is_file() else None
    if platform.system() == "Windows":
        for env_name in ("PROGRAMFILES", "PROGRAMFILES(X86)"):
            base = os.environ.get(env_name)
            if not base:
                continue
            candidate = Path(base) / "LibreOffice" / "program" / "soffice.exe"
            if candidate.is_file():
                return candidate
    return None


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
def temporary_profile():
    with temporary_directory("awaken-ppt-lo-profile-") as directory:
        yield directory


def run_soffice(arguments: Sequence[str], *, timeout: int, profile: Path | None = None) -> subprocess.CompletedProcess[str]:
    executable = find_soffice()
    if executable is None:
        raise FileNotFoundError("未找到 LibreOffice（soffice/libreoffice）")
    if profile is None:
        with temporary_profile() as generated:
            return run_soffice(arguments, timeout=timeout, profile=generated)
    profile.mkdir(parents=True, exist_ok=True)
    return subprocess.run(
        [
            str(executable),
            "--headless",
            "--nologo",
            "--norestore",
            f"-env:UserInstallation={profile.resolve().as_uri()}",
            *arguments,
        ],
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
        env=office_env(),
    )
