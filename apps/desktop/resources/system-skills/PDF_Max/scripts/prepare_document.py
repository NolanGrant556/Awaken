#!/usr/bin/env python3
"""从 Skill 自带资产准备工作区内的 Typst 项目。"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path


SKILL_ROOT = Path(__file__).resolve().parent.parent
ASSET_ROOT = SKILL_ROOT / "assets"


def within(path: Path, root: Path) -> bool:
    try:
        return os.path.commonpath([path, root]) == str(root)
    except ValueError:
        return False


def ensure_output(output: Path, workspace: Path, names: tuple[str, ...], force: bool) -> None:
    if not within(output, workspace):
        raise ValueError("项目输出目录必须位于工作区内")
    output.mkdir(parents=True, exist_ok=True)
    conflicts = [output / name for name in names if (output / name).exists()]
    if conflicts and not force:
        raise ValueError("拒绝覆盖已有文件：" + ", ".join(str(path) for path in conflicts))


def read_asset(name: str) -> str:
    path = ASSET_ROOT / name
    if not path.is_file():
        raise ValueError(f"缺少 Skill 资产：{path}")
    return path.read_text(encoding="utf-8")


def language_values(language: str, region: str | None) -> dict[str, str]:
    if language == "zh":
        return {
            "language": "zh",
            "region": region or "cn",
            "author_label": "作者",
            "date_label": "日期",
            "outline_title": "目录",
            "intro_heading": "引言",
            "body_placeholder": "请从这里开始撰写正文。",
        }
    return {
        "language": "en",
        "region": region or "us",
        "author_label": "Author",
        "date_label": "Date",
        "outline_title": "Contents",
        "intro_heading": "Introduction",
        "body_placeholder": "Start writing the document here.",
    }


def document_config(args: argparse.Namespace) -> dict[str, object]:
    localized = language_values(args.language, args.region)
    body_fonts = [args.body_font]
    heading_fonts = [args.heading_font or args.body_font]
    if args.cjk_font:
        body_fonts.append(args.cjk_font)
        heading_fonts.append(args.cjk_font)
    if args.language == "zh" and not args.cjk_font:
        raise ValueError("中文文档必须先通过 `typst fonts` 选择并传入 --cjk-font")
    return {
        "title": getattr(args, "title", ""),
        "subtitle": getattr(args, "subtitle", ""),
        "author": getattr(args, "author", ""),
        "language": localized["language"],
        "region": localized["region"],
        "body_fonts": body_fonts,
        "heading_fonts": heading_fonts,
        "author_label": localized["author_label"],
        "date_label": localized["date_label"],
        "outline_title": localized["outline_title"],
        "intro_heading": localized["intro_heading"],
        "body_placeholder": localized["body_placeholder"],
    }


def write_text(path: Path, value: str) -> None:
    temporary = path.with_name(path.name + ".tmp")
    temporary.write_text(value, encoding="utf-8")
    os.replace(temporary, path)


def write_manifest(output: Path, payload: dict[str, object]) -> None:
    write_text(
        output / ".typst-document.json",
        json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
    )


def prepare_report(args: argparse.Namespace) -> Path:
    workspace = args.workspace_root.resolve()
    output = args.output_dir.resolve()
    generated = ("report-theme.typ", "main.typ", ".typst-config.json", ".typst-document.json")
    ensure_output(output, workspace, generated, args.force)
    config = document_config(args)
    write_text(output / "report-theme.typ", read_asset("report-theme.typ"))
    write_text(output / "main.typ", read_asset("report-entry.typ"))
    write_text(
        output / ".typst-config.json",
        json.dumps(config, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
    )
    write_manifest(
        output,
        {
            "schema_version": 1,
            "mode": "report",
            "entry": "main.typ",
            "language": args.language,
            "region": config["region"],
            "fonts": {
                "body": args.body_font,
                "heading": args.heading_font or args.body_font,
                "cjk": args.cjk_font,
            },
            "metadata": {"title": args.title, "subtitle": args.subtitle, "author": args.author},
        },
    )
    return output / "main.typ"


def prepare_markdown(args: argparse.Namespace) -> Path:
    source = args.source.resolve()
    if not source.is_file() or source.suffix.lower() not in {".md", ".markdown"}:
        raise ValueError("Markdown 来源必须是存在的 .md 或 .markdown 文件")
    workspace = args.workspace_root.resolve()
    output = args.output_dir.resolve()
    generated = (
        "report-theme.typ",
        "main.typ",
        "source.md",
        ".typst-config.json",
        ".typst-document.json",
    )
    ensure_output(output, workspace, generated, args.force)
    config = document_config(args)
    config["markdown_source"] = "source.md"
    write_text(output / "report-theme.typ", read_asset("report-theme.typ"))
    write_text(output / "source.md", source.read_text(encoding="utf-8"))
    write_text(output / "main.typ", read_asset("markdown-entry.typ"))
    write_text(
        output / ".typst-config.json",
        json.dumps(config, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
    )
    write_manifest(
        output,
        {
            "schema_version": 1,
            "mode": "markdown",
            "entry": "main.typ",
            "source": {"original_path": str(source), "prepared_path": "source.md"},
            "language": args.language,
            "fonts": {"body": args.body_font, "cjk": args.cjk_font},
        },
    )
    return output / "main.typ"


def add_style_arguments(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--language", choices=("zh", "en"), default="zh")
    parser.add_argument("--region")
    parser.add_argument("--body-font", default="Libertinus Serif")
    parser.add_argument("--heading-font")
    parser.add_argument("--cjk-font", default="")
    parser.add_argument("--workspace-root", type=Path, default=Path.cwd())
    parser.add_argument("--force", action="store_true")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="准备 Awaken Typst PDF 项目")
    subparsers = parser.add_subparsers(dest="mode", required=True)
    report = subparsers.add_parser("report", help="准备原生专业报告")
    report.add_argument("output_dir", type=Path)
    report.add_argument("--title", required=True)
    report.add_argument("--subtitle", default="")
    report.add_argument("--author", default="")
    add_style_arguments(report)
    report.set_defaults(handler=prepare_report)

    markdown = subparsers.add_parser("markdown", help="准备 Markdown 转 PDF 项目")
    markdown.add_argument("source", type=Path)
    markdown.add_argument("output_dir", type=Path)
    markdown.add_argument("--title", default="")
    markdown.add_argument("--subtitle", default="")
    markdown.add_argument("--author", default="")
    add_style_arguments(markdown)
    markdown.set_defaults(handler=prepare_markdown)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    try:
        entry = args.handler(args)
    except (OSError, ValueError) as exc:
        print(f"错误：{exc}", file=sys.stderr)
        return 2
    print(json.dumps({"entry": str(entry), "status": "prepared"}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
