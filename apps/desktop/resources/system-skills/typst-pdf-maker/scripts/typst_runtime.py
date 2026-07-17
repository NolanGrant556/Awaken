#!/usr/bin/env python3
"""为 Awaken Skill 解析或安装固定版本的 Typst CLI。"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import urllib.error
import urllib.request
import zipfile
from pathlib import Path, PurePosixPath


TYPST_VERSION = "0.15.0"
RELEASE_ROOT = f"https://github.com/typst/typst/releases/download/v{TYPST_VERSION}"

ASSETS: dict[tuple[str, str], dict[str, str]] = {
    ("darwin", "arm64"): {
        "name": "typst-aarch64-apple-darwin.tar.xz",
        "sha256": "fe53838737abf93a774495952a1a797b4686e9c4a21c2d99b9fdf77f46cc3572",
        "member": "typst",
    },
    ("darwin", "x64"): {
        "name": "typst-x86_64-apple-darwin.tar.xz",
        "sha256": "30210c7c539c7954db94c063cd98b43fd0a0cad285d656dbbce2a40aee2e79be",
        "member": "typst",
    },
    ("windows", "arm64"): {
        "name": "typst-aarch64-pc-windows-msvc.zip",
        "sha256": "6085084db3e3aa025c3c8dfc1adf1c758e2febd08e574182fa4189ad79124293",
        "member": "typst.exe",
    },
    ("windows", "x64"): {
        "name": "typst-x86_64-pc-windows-msvc.zip",
        "sha256": "66ae7f0907b4b9afed5c7d6cb9b21e07f0f3c3d4e293ba3e0026a54d88202fe9",
        "member": "typst.exe",
    },
}


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


def normalized_arch(value: str | None = None) -> str:
    raw = (value or platform.machine()).lower()
    if raw in {"arm64", "aarch64"}:
        return "arm64"
    if raw in {"x86_64", "amd64", "x64"}:
        return "x64"
    raise RuntimeErrorWithCode(f"当前版本不支持此 CPU 架构：{raw}")


def selected_asset(
    platform_name: str | None = None,
    machine: str | None = None,
) -> dict[str, str]:
    key = (normalized_platform(platform_name), normalized_arch(machine))
    try:
        return dict(ASSETS[key])
    except KeyError as exc:
        raise RuntimeErrorWithCode(f"没有适用于 {key[0]}/{key[1]} 的 Typst 安装资产") from exc


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


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def download(url: str, destination: Path) -> None:
    request = urllib.request.Request(
        url,
        headers={"User-Agent": "Awaken-Typst-PDF-Skill/1.0.0"},
    )
    try:
        with urllib.request.urlopen(request, timeout=120) as response:
            if getattr(response, "status", 200) != 200:
                raise RuntimeErrorWithCode(f"Typst 下载失败，HTTP {response.status}")
            with destination.open("wb") as output:
                shutil.copyfileobj(response, output, length=1024 * 1024)
    except (OSError, urllib.error.URLError) as exc:
        raise RuntimeErrorWithCode(f"Typst 下载失败：{exc}") from exc


def safe_member_name(name: str, expected_basename: str) -> bool:
    normalized = name.replace("\\", "/")
    path = PurePosixPath(normalized)
    return (
        not path.is_absolute()
        and ".." not in path.parts
        and path.name == expected_basename
        and len(path.parts) >= 2
    )


def extract_executable(archive: Path, destination: Path, expected_basename: str) -> None:
    matches: list[tuple[str, object]] = []
    if archive.suffix.lower() == ".zip":
        with zipfile.ZipFile(archive) as bundle:
            for info in bundle.infolist():
                if not info.is_dir() and safe_member_name(info.filename, expected_basename):
                    matches.append((info.filename, info))
            if len(matches) != 1:
                raise RuntimeErrorWithCode("Typst ZIP 中没有唯一且安全的目标可执行文件")
            with bundle.open(matches[0][1]) as source, destination.open("wb") as output:
                shutil.copyfileobj(source, output)
        return

    with tarfile.open(archive, mode="r:xz") as bundle:
        for member in bundle.getmembers():
            if member.isfile() and safe_member_name(member.name, expected_basename):
                matches.append((member.name, member))
        if len(matches) != 1:
            raise RuntimeErrorWithCode("Typst TAR 中没有唯一且安全的目标可执行文件")
        source = bundle.extractfile(matches[0][1])
        if source is None:
            raise RuntimeErrorWithCode("无法读取 Typst 可执行文件")
        with source, destination.open("wb") as output:
            shutil.copyfileobj(source, output)


def atomic_write_json(path: Path, payload: dict[str, object]) -> None:
    temporary = path.with_name(path.name + ".tmp")
    temporary.write_text(
        json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    os.replace(temporary, path)


def install_runtime(force: bool = False) -> dict[str, object]:
    existing = resolve_runtime()
    if existing["path"] and not force:
        return existing

    root = awaken_home()
    target = managed_binary(root)
    tool_root = target.parent.parent
    target.parent.mkdir(parents=True, exist_ok=True)
    asset = selected_asset()
    url = f"{RELEASE_ROOT}/{asset['name']}"

    staging_root = Path(tempfile.mkdtemp(prefix=".install-", dir=tool_root))
    try:
        archive = staging_root / asset["name"]
        staged_binary = staging_root / asset["member"]
        download(url, archive)
        actual_hash = file_sha256(archive)
        if actual_hash != asset["sha256"]:
            raise RuntimeErrorWithCode(
                "Typst 安装包 SHA-256 校验失败；文件不会被安装"
            )
        extract_executable(archive, staged_binary, asset["member"])
        if normalized_platform() != "windows":
            staged_binary.chmod(
                stat.S_IRUSR | stat.S_IWUSR | stat.S_IXUSR | stat.S_IRGRP | stat.S_IXGRP | stat.S_IROTH | stat.S_IXOTH
            )
        version = probe_version(staged_binary)
        if not version or TYPST_VERSION not in version:
            raise RuntimeErrorWithCode(
                f"安装后的 Typst 版本验证失败：{version or '没有版本输出'}"
            )
        os.replace(staged_binary, target)
        metadata = {
            "schema_version": 1,
            "version": TYPST_VERSION,
            "asset": asset["name"],
            "sha256": asset["sha256"],
            "path": str(target),
        }
        atomic_write_json(tool_root / "installation.json", metadata)
        return {
            "path": str(target.resolve()),
            "version": version,
            "source": "awaken-managed",
            "installed": True,
        }
    finally:
        shutil.rmtree(staging_root, ignore_errors=True)


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
    parser = argparse.ArgumentParser(description="解析或安装 Awaken 使用的 Typst CLI")
    subparsers = parser.add_subparsers(dest="command", required=True)
    resolve_parser = subparsers.add_parser("resolve", help="检查固定路径与系统 PATH")
    resolve_parser.add_argument("--json", action="store_true")
    install_parser = subparsers.add_parser("install", help="安装固定受测版本")
    install_parser.add_argument("--force", action="store_true", help="忽略 PATH 中已有版本并安装受测版本")
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
