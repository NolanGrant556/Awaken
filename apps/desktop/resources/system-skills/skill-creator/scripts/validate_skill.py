#!/usr/bin/env python3
"""Preflight one Agent Skills directory before Awaken Core dry-run validation."""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path
from urllib.parse import unquote


NAME_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
MAX_FILES = 1000
MAX_BYTES = 50 << 20


def referenced_resources(body: str) -> list[str]:
    """Read explicit package paths without truncating Unicode or external paths.

    Kept in both portable Creator packages: neither installed package may import
    files from the other one's installation directory.
    """
    candidates: list[str] = []
    def collect(match: re.Match[str]) -> str:
        candidates.append(next(value for value in match.groups() if value is not None))
        return " "

    # Angle-bracket destinations support spaces; ordinary Markdown links may
    # carry a title. Code spans and quoted CLI arguments preserve spaces too.
    remaining = re.sub(r'\[[^\]\n]*\]\(\s*(?:<([^>\n]+)>|([^\s)]+)(?:\s+"[^"\n]*")?)\s*\)', collect, body)
    remaining = re.sub(r'(?<!`)`([^`\n]+)`(?!`)', collect, remaining)
    remaining = re.sub(r'"([^"\n]+)"|\'([^\'\n]+)\'', collect, remaining)
    candidates.extend(re.findall(
        r'(?<![\w/\\.$~:-])((?:\./)?(?:scripts|references|assets)/[^\s`"\'<>()[\]，。；、]+)', remaining
    ))
    paths: set[str] = set()
    for candidate in candidates:
        candidate = candidate.strip().removeprefix("${AWAKEN_SKILL_DIR}/").removeprefix("./")
        if not candidate.startswith(("scripts/", "references/", "assets/")):
            continue
        if any(marker in candidate for marker in ("${", "<", ">", "*")):
            continue
        candidate = unquote(candidate.split("#", 1)[0]).rstrip(".,;:，。；")
        paths.add(candidate)
    return sorted(paths)


def scalar_value(lines: list[str], key: str) -> str | None:
    prefix = key + ":"
    for index, line in enumerate(lines):
        if not line.startswith(prefix):
            continue
        raw = line[len(prefix) :].strip()
        if raw in {"|", ">", "|-", ">-", "|+", ">+"}:
            values: list[str] = []
            for follow in lines[index + 1 :]:
                if follow.startswith((" ", "\t")):
                    values.append(follow.strip())
                    continue
                break
            return " ".join(value for value in values if value)
        if raw.startswith('"'):
            try:
                value = json.loads(raw)
                return value if isinstance(value, str) else None
            except json.JSONDecodeError:
                return None
        if raw.startswith("'") and raw.endswith("'"):
            return raw[1:-1].replace("''", "'")
        return raw.split(" #", 1)[0].strip()
    return None


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("skill_root")
    args = parser.parse_args()

    root = Path(args.skill_root).expanduser().resolve()
    errors: list[str] = []
    warnings: list[str] = []
    files: list[str] = []
    total_bytes = 0

    if not root.is_dir():
        errors.append(f"skill root is not a directory: {root}")
    else:
        for current, dirs, names in os.walk(root, followlinks=False):
            current_path = Path(current)
            for name in [*dirs, *names]:
                candidate = current_path / name
                if candidate.is_symlink():
                    errors.append(f"symlink is not allowed: {candidate.relative_to(root)}")
            for name in names:
                candidate = current_path / name
                if candidate.is_symlink() or not candidate.is_file():
                    continue
                rel = candidate.relative_to(root).as_posix()
                files.append(rel)
                total_bytes += candidate.stat().st_size

    if len(files) > MAX_FILES:
        errors.append(f"package contains {len(files)} files; maximum is {MAX_FILES}")
    if total_bytes > MAX_BYTES:
        errors.append(f"package contains {total_bytes} bytes; maximum is {MAX_BYTES}")

    skill_files = [path for path in files if Path(path).name.lower() == "skill.md"]
    if skill_files != ["SKILL.md"]:
        errors.append("package must contain exactly one root SKILL.md")

    entry = root / "SKILL.md"
    text = ""
    if entry.is_file() and not entry.is_symlink():
        try:
            text = entry.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            errors.append("SKILL.md must be UTF-8")

    if text:
        lines = text.splitlines()
        if not lines or lines[0].strip() != "---":
            errors.append("SKILL.md must start with YAML frontmatter")
            frontmatter: list[str] = []
            body = ""
        else:
            try:
                close = next(index for index, line in enumerate(lines[1:], 1) if line.strip() == "---")
                frontmatter = lines[1:close]
                body = "\n".join(lines[close + 1 :]).strip()
            except StopIteration:
                errors.append("SKILL.md frontmatter is missing its closing delimiter")
                frontmatter = []
                body = ""

        name = scalar_value(frontmatter, "name")
        description = scalar_value(frontmatter, "description")
        if not name:
            errors.append("frontmatter name is required")
        else:
            if len(name) > 64 or not NAME_RE.fullmatch(name):
                errors.append("name must be 1-64 lowercase ASCII letters, digits, and single hyphens")
            if name != root.name:
                errors.append(f"name {name!r} must match parent directory {root.name!r}")
        if not description:
            errors.append("frontmatter description is required")
        elif len(description) > 1024:
            errors.append("description must not exceed 1024 characters")
        if not body:
            warnings.append("SKILL.md body is empty")

        referenced = referenced_resources(body)
        for rel in referenced:
            if not (root / rel).resolve().is_relative_to(root):
                errors.append(f"referenced package resource escapes package: {rel}")
            elif not (root / rel).exists():
                errors.append(f"referenced package resource does not exist: {rel}")

    for directory in ("scripts", "references", "assets"):
        candidate = root / directory
        if candidate.is_dir() and not any(item.is_file() for item in candidate.rglob("*")):
            warnings.append(f"empty resource directory should be filled or removed: {directory}/")

    result = {
        "valid": not errors,
        "skill_root": str(root),
        "file_count": len(files),
        "total_bytes": total_bytes,
        "files": sorted(files),
        "errors": errors,
        "warnings": warnings,
        "next": "run skill_install with validate_only=true" if not errors else "fix errors and rerun preflight",
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
