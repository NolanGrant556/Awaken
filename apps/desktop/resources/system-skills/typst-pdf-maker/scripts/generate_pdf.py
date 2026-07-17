#!/usr/bin/env python3
"""使用指定 Typst CLI 原子生成工作区内的 PDF。"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path


def is_within(path: Path, root: Path) -> bool:
    try:
        return os.path.commonpath([path, root]) == str(root)
    except ValueError:
        return False


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="严格编译 Typst 文档并生成 PDF")
    parser.add_argument("input_file", type=Path)
    parser.add_argument("--typst-bin", type=Path, required=True)
    parser.add_argument("-o", "--output", type=Path)
    parser.add_argument("--workspace-root", type=Path, default=Path.cwd())
    parser.add_argument("--strict", action="store_true", help="把 Typst warning 视为失败")
    parser.add_argument("--timeout", type=int, default=300)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    binary = args.typst_bin.expanduser()
    if not binary.is_absolute() or not binary.is_file():
        print("错误：--typst-bin 必须指向存在的绝对文件路径", file=sys.stderr)
        return 2

    source = args.input_file.expanduser().resolve()
    if not source.is_file() or source.suffix.lower() != ".typ":
        print("错误：输入文件必须是存在的 .typ 文件", file=sys.stderr)
        return 2

    workspace = args.workspace_root.expanduser().resolve()
    if not workspace.is_dir():
        print("错误：工作区目录不存在", file=sys.stderr)
        return 2

    output = (args.output or source.with_suffix(".pdf")).expanduser().resolve()
    if output.suffix.lower() != ".pdf" or not is_within(output, workspace):
        print("错误：最终 PDF 必须位于工作区内并使用 .pdf 扩展名", file=sys.stderr)
        return 2
    output.parent.mkdir(parents=True, exist_ok=True)
    staged = output.with_name(f".{output.stem}.awaken-build.pdf")
    staged.unlink(missing_ok=True)

    command = [str(binary), "compile", str(source), str(staged)]
    try:
        completed = subprocess.run(
            command,
            capture_output=True,
            text=True,
            timeout=args.timeout,
        )
    except subprocess.TimeoutExpired:
        staged.unlink(missing_ok=True)
        print(f"错误：Typst 编译超过 {args.timeout} 秒", file=sys.stderr)
        return 124
    except OSError as exc:
        staged.unlink(missing_ok=True)
        print(f"错误：无法启动 Typst：{exc}", file=sys.stderr)
        return 126

    diagnostics = completed.stderr.strip()
    if diagnostics:
        print(diagnostics, file=sys.stderr)
    if completed.returncode != 0:
        staged.unlink(missing_ok=True)
        return completed.returncode
    if args.strict and "warning:" in diagnostics.lower():
        staged.unlink(missing_ok=True)
        print("错误：严格模式拒绝包含 warning 的构建", file=sys.stderr)
        return 2
    try:
        with staged.open("rb") as handle:
            signature = handle.read(5)
    except OSError as exc:
        print(f"错误：无法读取 Typst 输出：{exc}", file=sys.stderr)
        return 2
    if signature != b"%PDF-":
        staged.unlink(missing_ok=True)
        print("错误：Typst 输出缺少 PDF 文件签名", file=sys.stderr)
        return 2

    os.replace(staged, output)
    print(json_result(output))
    return 0


def json_result(output: Path) -> str:
    import json

    return json.dumps(
        {"output": str(output), "bytes": output.stat().st_size, "status": "compiled"},
        ensure_ascii=False,
        sort_keys=True,
    )


if __name__ == "__main__":
    raise SystemExit(main())
