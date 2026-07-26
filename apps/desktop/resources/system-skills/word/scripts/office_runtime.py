#!/usr/bin/env python3
"""Awaken Office Skill 共用的 LibreOffice/Poppler 进程辅助函数。"""

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
def temporary_profile(prefix: str = "awaken-lo-profile-"):
    with tempfile.TemporaryDirectory(prefix=prefix, ignore_cleanup_errors=True) as directory:
        yield Path(directory)


def run_soffice(
    arguments: Sequence[str],
    *,
    timeout: int,
    profile: Path | None = None,
) -> subprocess.CompletedProcess[str]:
    executable = find_soffice()
    if executable is None:
        raise FileNotFoundError("未找到 LibreOffice（soffice/libreoffice）")
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
