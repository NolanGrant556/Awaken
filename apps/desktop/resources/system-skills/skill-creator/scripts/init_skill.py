#!/usr/bin/env python3
"""Create a strict Agent Skills candidate without touching installed roots."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path


NAME_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
RESOURCE_DIRS = {"scripts", "references", "assets"}


def parse_resources(raw: str) -> list[str]:
    if not raw.strip():
        return []
    resources = [item.strip() for item in raw.split(",") if item.strip()]
    unknown = sorted(set(resources) - RESOURCE_DIRS)
    if unknown:
        raise argparse.ArgumentTypeError(
            "unsupported standard resource directories: " + ", ".join(unknown)
        )
    return list(dict.fromkeys(resources))


def fail(message: str) -> int:
    print(json.dumps({"ok": False, "error": message}, ensure_ascii=False))
    return 1


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("name", help="Skill name and directory name")
    parser.add_argument("--path", required=True, help="Parent authoring directory")
    parser.add_argument("--description", required=True, help="What the Skill does and when to use it")
    parser.add_argument(
        "--resources",
        default="",
        type=parse_resources,
        help="Comma-separated subset of scripts,references,assets",
    )
    args = parser.parse_args()

    name = args.name.strip()
    description = args.description.strip()
    if len(name) > 64 or not NAME_RE.fullmatch(name):
        return fail("name must be 1-64 lowercase ASCII letters, digits, and single hyphens")
    if not description or len(description) > 1024:
        return fail("description must be 1-1024 characters and explain what and when")

    parent = Path(args.path).expanduser().resolve()
    target = parent / name
    if target.exists() or target.is_symlink():
        return fail(f"target already exists: {target}")

    try:
        target.mkdir(parents=True, mode=0o700)
        title = " ".join(part.capitalize() for part in name.split("-"))
        skill_md = (
            "---\n"
            f"name: {name}\n"
            f"description: {json.dumps(description, ensure_ascii=False)}\n"
            "---\n\n"
            f"# {title}\n\n"
            f"{description}\n\n"
            "## Workflow\n\n"
            "Replace this starter with the real workflow, decision points, resource routes, and acceptance criteria.\n"
        )
        (target / "SKILL.md").write_text(skill_md, encoding="utf-8")
        for resource in args.resources:
            (target / resource).mkdir(mode=0o700)
    except Exception as exc:  # pragma: no cover - defensive CLI boundary
        return fail(f"create candidate: {exc}")

    print(
        json.dumps(
            {
                "ok": True,
                "skill_root": str(target),
                "entrypoint": str(target / "SKILL.md"),
                "resource_directories": args.resources,
                "next": "fill or remove empty resource directories, then run validate_skill.py",
            },
            ensure_ascii=False,
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
