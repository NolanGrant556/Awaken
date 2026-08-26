#!/usr/bin/env python3
"""Preflight an Agent Plugins 1.0.0 candidate before Core validation."""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path


PLUGIN_SCHEMA = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
MCP_SCHEMA = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
PLUGIN_FIELDS = {
    "$schema", "name", "version", "description", "author", "homepage",
    "repository", "license", "keywords", "extensions",
}
PLUGIN_NAME_RE = re.compile(r"^(?!.*(?:--|\.\.))[a-z0-9](?:[a-z0-9.-]{0,62}[a-z0-9])?$")
SKILL_NAME_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
RESOURCE_RE = re.compile(r"(?<![A-Za-z0-9_.-])((?:scripts|references|assets)/[A-Za-z0-9_.\-/]+)")
MAX_FILES = 10_000
MAX_BYTES = 500 << 20


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


def validate_skill(root: Path, errors: list[str], warnings: list[str]) -> None:
    entry = root / "SKILL.md"
    label = root.relative_to(root.parents[1]).as_posix()
    if not entry.is_file() or entry.is_symlink():
        errors.append(f"{label}: missing regular SKILL.md")
        return
    try:
        text = entry.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        errors.append(f"{label}: SKILL.md must be UTF-8")
        return
    lines = text.splitlines()
    if not lines or lines[0].strip() != "---":
        errors.append(f"{label}: SKILL.md must start with YAML frontmatter")
        return
    try:
        close = next(index for index, line in enumerate(lines[1:], 1) if line.strip() == "---")
    except StopIteration:
        errors.append(f"{label}: frontmatter is missing its closing delimiter")
        return
    frontmatter = lines[1:close]
    body = "\n".join(lines[close + 1 :]).strip()
    name = scalar_value(frontmatter, "name")
    description = scalar_value(frontmatter, "description")
    if not name or len(name) > 64 or not SKILL_NAME_RE.fullmatch(name):
        errors.append(f"{label}: invalid or missing Agent Skills name")
    elif name != root.name:
        errors.append(f"{label}: name {name!r} must match directory {root.name!r}")
    if not description or len(description) > 1024:
        errors.append(f"{label}: description must be 1-1024 characters")
    if not body:
        warnings.append(f"{label}: SKILL.md body is empty")
    for rel in sorted({match.rstrip(".,;:)") for match in RESOURCE_RE.findall(body)}):
        if not (root / rel).exists():
            errors.append(f"{label}: referenced resource does not exist: {rel}")
    for directory in ("scripts", "references", "assets"):
        candidate = root / directory
        if candidate.is_dir() and not any(item.is_file() for item in candidate.rglob("*")):
            warnings.append(f"{label}: empty resource directory should be filled or removed: {directory}/")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("plugin_root")
    args = parser.parse_args()
    root = Path(args.plugin_root).expanduser().resolve()
    errors: list[str] = []
    warnings: list[str] = []
    files: list[str] = []
    total_bytes = 0

    if not root.is_dir():
        errors.append(f"plugin root is not a directory: {root}")
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
                files.append(candidate.relative_to(root).as_posix())
                total_bytes += candidate.stat().st_size

    if len(files) > MAX_FILES:
        errors.append(f"package contains {len(files)} files; maximum is {MAX_FILES}")
    if total_bytes > MAX_BYTES:
        errors.append(f"package contains {total_bytes} bytes; maximum is {MAX_BYTES}")

    manifest_path = root / "plugin.json"
    manifest: dict[str, object] | None = None
    if not manifest_path.is_file() or manifest_path.is_symlink():
        errors.append("root plugin.json is required and must be a regular file")
    else:
        try:
            parsed = json.loads(manifest_path.read_text(encoding="utf-8"))
            manifest = parsed if isinstance(parsed, dict) else None
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            errors.append(f"plugin.json is invalid: {exc}")
        if manifest is None and not errors:
            errors.append("plugin.json must contain an object")
    if manifest is not None:
        unknown = sorted(set(manifest) - PLUGIN_FIELDS)
        if unknown:
            warnings.append("unknown plugin.json fields are ignored: " + ", ".join(unknown))
        if manifest.get("$schema") != PLUGIN_SCHEMA:
            errors.append("plugin.json must target Agent Plugins 1.0.0")
        name = manifest.get("name")
        if not isinstance(name, str) or len(name) > 64 or not PLUGIN_NAME_RE.fullmatch(name):
            errors.append("plugin.json name does not satisfy Agent Plugins 1.0.0")
        extensions = manifest.get("extensions")
        if extensions is not None and not isinstance(extensions, dict):
            warnings.append("non-object extensions is ignored by the standard")

    skill_count = 0
    skills_root = root / "skills"
    if skills_root.exists() and not skills_root.is_dir():
        errors.append("skills must be a directory when present")
    elif skills_root.is_dir():
        for child in sorted(skills_root.iterdir(), key=lambda item: item.name):
            if child.is_dir() and not child.is_symlink():
                if (child / "SKILL.md").exists():
                    skill_count += 1
                    validate_skill(child, errors, warnings)
                else:
                    warnings.append(f"skills/{child.name}: ignored because SKILL.md is missing")
            else:
                warnings.append(f"skills/{child.name}: ignored because it is not a Skill directory")

    server_count = 0
    mcp_path = root / "mcp.json"
    if mcp_path.exists():
        if not mcp_path.is_file() or mcp_path.is_symlink():
            errors.append("mcp.json must be a regular file")
        else:
            try:
                mcp = json.loads(mcp_path.read_text(encoding="utf-8"))
            except (UnicodeDecodeError, json.JSONDecodeError) as exc:
                errors.append(f"mcp.json is invalid: {exc}")
                mcp = None
            if not isinstance(mcp, dict):
                if mcp is not None:
                    errors.append("mcp.json must contain an object")
            else:
                if set(mcp) != {"$schema", "mcpServers"}:
                    errors.append("mcp.json may only contain $schema and mcpServers")
                if mcp.get("$schema") != MCP_SCHEMA:
                    errors.append("mcp.json must target Agent Plugins 1.0.0")
                servers = mcp.get("mcpServers")
                if not isinstance(servers, dict):
                    errors.append("mcpServers must be an object")
                else:
                    server_count = len(servers)

    if skill_count == 0 and server_count == 0:
        warnings.append("plugin contains no discovered Skill or configured MCP server")

    result = {
        "valid": not errors,
        "plugin_root": str(root),
        "file_count": len(files),
        "total_bytes": total_bytes,
        "skill_count": skill_count,
        "server_count": server_count,
        "files": sorted(files),
        "errors": errors,
        "warnings": warnings,
        "next": "run plugin action=validate" if not errors else "fix errors and rerun preflight",
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
