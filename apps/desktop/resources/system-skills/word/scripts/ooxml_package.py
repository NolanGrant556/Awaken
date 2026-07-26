#!/usr/bin/env python3
"""安全解包和重新打包 DOCX/DOTX，不接受路径逃逸或符号链接。"""

from __future__ import annotations

import argparse
import json
import os
import stat
import zipfile
from pathlib import Path, PurePosixPath


REQUIRED_PARTS = {"[Content_Types].xml", "_rels/.rels", "word/document.xml"}


def normalized_member(name: str) -> PurePosixPath:
    if not name or "\\" in name or name.startswith("/"):
        raise ValueError(f"非法 OOXML 路径: {name!r}")
    path = PurePosixPath(name)
    if any(part in ("", ".", "..") for part in path.parts):
        raise ValueError(f"非法 OOXML 路径: {name!r}")
    return path


def is_link_or_special(info: zipfile.ZipInfo) -> bool:
    mode = info.external_attr >> 16
    if mode == 0:
        return False
    kind = stat.S_IFMT(mode)
    return kind not in (0, stat.S_IFREG, stat.S_IFDIR)


def safe_extract(archive: Path, destination: Path) -> list[str]:
    if destination.exists() and any(destination.iterdir()):
        raise ValueError(f"目标目录不是空目录: {destination}")
    destination.mkdir(parents=True, exist_ok=True)
    seen: set[str] = set()
    with zipfile.ZipFile(archive) as package:
        for info in package.infolist():
            member = normalized_member(info.filename.rstrip("/"))
            key = member.as_posix()
            if key in seen:
                raise ValueError(f"OOXML 包含重复部件: {key}")
            seen.add(key)
            if is_link_or_special(info):
                raise ValueError(f"OOXML 包含符号链接或特殊文件: {key}")
            target = destination.joinpath(*member.parts)
            target.resolve().relative_to(destination.resolve())
            if info.is_dir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            with package.open(info) as source, target.open("wb") as output:
                while chunk := source.read(1024 * 1024):
                    output.write(chunk)
    missing = sorted(REQUIRED_PARTS - seen)
    if missing:
        raise ValueError(f"DOCX 缺少必需部件: {', '.join(missing)}")
    return sorted(seen)


def pack_directory(source: Path, output: Path) -> list[str]:
    if not source.is_dir() or source.is_symlink():
        raise ValueError(f"源必须是真实目录: {source}")
    files: list[Path] = []
    for root, directories, names in os.walk(source, followlinks=False):
        root_path = Path(root)
        for directory in directories:
            candidate = root_path / directory
            if candidate.is_symlink():
                raise ValueError(f"目录包含符号链接: {candidate}")
        for name in names:
            candidate = root_path / name
            metadata = candidate.lstat()
            if candidate.is_symlink() or not stat.S_ISREG(metadata.st_mode):
                raise ValueError(f"目录包含链接或特殊文件: {candidate}")
            files.append(candidate)
    relative = {path.relative_to(source).as_posix() for path in files}
    missing = sorted(REQUIRED_PARTS - relative)
    if missing:
        raise ValueError(f"解包目录缺少必需部件: {', '.join(missing)}")
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_name(f".{output.name}.awaken-tmp")
    if temporary.exists():
        raise ValueError(f"临时输出已存在，请先处理: {temporary}")
    try:
        with zipfile.ZipFile(temporary, "w", compression=zipfile.ZIP_DEFLATED) as package:
            for path in sorted(files, key=lambda item: item.relative_to(source).as_posix()):
                package.write(path, path.relative_to(source).as_posix())
        temporary.replace(output)
    finally:
        if temporary.exists():
            temporary.unlink()
    return sorted(relative)


def main() -> int:
    parser = argparse.ArgumentParser(description="安全解包/打包 DOCX")
    subparsers = parser.add_subparsers(dest="command", required=True)
    unpack = subparsers.add_parser("unpack")
    unpack.add_argument("input", type=Path)
    unpack.add_argument("output", type=Path)
    pack = subparsers.add_parser("pack")
    pack.add_argument("input", type=Path)
    pack.add_argument("output", type=Path)
    args = parser.parse_args()
    try:
        files = (
            safe_extract(args.input.resolve(), args.output.resolve())
            if args.command == "unpack"
            else pack_directory(args.input.resolve(), args.output.resolve())
        )
        print(json.dumps({"status": "success", "files": len(files)}, ensure_ascii=False))
        return 0
    except Exception as error:
        print(json.dumps({"status": "failed", "error": str(error)}, ensure_ascii=False))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
