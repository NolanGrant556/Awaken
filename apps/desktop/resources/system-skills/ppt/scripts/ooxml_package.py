#!/usr/bin/env python3
"""安全解包和重新打包 PPTX/POTX。"""

from __future__ import annotations

import argparse
import json
import os
import stat
import zipfile
from pathlib import Path, PurePosixPath


REQUIRED = {
    "[Content_Types].xml",
    "_rels/.rels",
    "ppt/presentation.xml",
    "ppt/_rels/presentation.xml.rels",
}


def normalized(name: str) -> PurePosixPath:
    if not name or name.startswith("/") or "\\" in name:
        raise ValueError(f"非法 OOXML 路径: {name!r}")
    path = PurePosixPath(name.rstrip("/"))
    if any(part in ("", ".", "..") for part in path.parts):
        raise ValueError(f"非法 OOXML 路径: {name!r}")
    return path


def safe_extract(archive: Path, destination: Path) -> list[str]:
    if destination.exists() and any(destination.iterdir()):
        raise ValueError(f"目标目录不是空目录: {destination}")
    destination.mkdir(parents=True, exist_ok=True)
    seen: set[str] = set()
    with zipfile.ZipFile(archive) as package:
        for info in package.infolist():
            path = normalized(info.filename)
            name = path.as_posix()
            if name in seen:
                raise ValueError(f"重复部件: {name}")
            seen.add(name)
            mode = info.external_attr >> 16
            kind = stat.S_IFMT(mode) if mode else 0
            if kind not in (0, stat.S_IFREG, stat.S_IFDIR):
                raise ValueError(f"链接或特殊部件: {name}")
            target = destination.joinpath(*path.parts)
            target.resolve().relative_to(destination.resolve())
            if info.is_dir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            with package.open(info) as source, target.open("wb") as output:
                while chunk := source.read(1024 * 1024):
                    output.write(chunk)
    missing = sorted(REQUIRED - seen)
    if missing:
        raise ValueError(f"PPTX 缺少必需部件: {', '.join(missing)}")
    return sorted(seen)


def pack_directory(source: Path, output: Path) -> list[str]:
    if not source.is_dir() or source.is_symlink():
        raise ValueError(f"源必须是真实目录: {source}")
    files: list[Path] = []
    for root, directories, names in os.walk(source, followlinks=False):
        root_path = Path(root)
        if any((root_path / name).is_symlink() for name in directories):
            raise ValueError("目录包含符号链接")
        for name in names:
            candidate = root_path / name
            metadata = candidate.lstat()
            if candidate.is_symlink() or not stat.S_ISREG(metadata.st_mode):
                raise ValueError(f"目录包含链接或特殊文件: {candidate}")
            files.append(candidate)
    names = {path.relative_to(source).as_posix() for path in files}
    missing = sorted(REQUIRED - names)
    if missing:
        raise ValueError(f"解包目录缺少必需部件: {', '.join(missing)}")
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_name(f".{output.name}.awaken-tmp")
    if temporary.exists():
        raise ValueError(f"临时输出已存在: {temporary}")
    try:
        with zipfile.ZipFile(temporary, "w", compression=zipfile.ZIP_DEFLATED) as package:
            for path in sorted(files, key=lambda item: item.relative_to(source).as_posix()):
                package.write(path, path.relative_to(source).as_posix())
        temporary.replace(output)
    finally:
        if temporary.exists():
            temporary.unlink()
    return sorted(names)


def main() -> int:
    parser = argparse.ArgumentParser(description="安全解包/打包 PPTX")
    subparsers = parser.add_subparsers(dest="command", required=True)
    unpack = subparsers.add_parser("unpack")
    unpack.add_argument("input", type=Path)
    unpack.add_argument("output", type=Path)
    pack = subparsers.add_parser("pack")
    pack.add_argument("input", type=Path)
    pack.add_argument("output", type=Path)
    args = parser.parse_args()
    try:
        files = safe_extract(args.input.resolve(), args.output.resolve()) if args.command == "unpack" else pack_directory(args.input.resolve(), args.output.resolve())
        print(json.dumps({"status": "success", "files": len(files)}, ensure_ascii=False))
        return 0
    except Exception as error:
        print(json.dumps({"status": "failed", "error": str(error)}, ensure_ascii=False))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
