#!/usr/bin/env python3
"""Create an Agent Plugins 1.0.0 candidate without touching Awaken's store."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path


PLUGIN_NAME_RE = re.compile(r"^(?!.*(?:--|\.\.))[a-z0-9](?:[a-z0-9.-]{0,62}[a-z0-9])?$")
SKILL_NAME_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
RESOURCE_DIRS = {"scripts", "references", "assets"}
PLUGIN_SCHEMA = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
MCP_SCHEMA = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"


def fail(message: str) -> int:
    print(json.dumps({"ok": False, "error": message}, ensure_ascii=False))
    return 1


def assignment(raw: str, label: str) -> tuple[str, str]:
    name, separator, value = raw.partition("=")
    name = name.strip()
    value = value.strip()
    if not separator or not name or not value:
        raise argparse.ArgumentTypeError(f"{label} must use name=value")
    return name, value


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("name")
    parser.add_argument("--path", required=True, help="Parent authoring directory")
    parser.add_argument("--description", required=True)
    parser.add_argument("--skill", action="append", default=[], help="Repeat name=description")
    parser.add_argument(
        "--skill-resources",
        action="append",
        default=[],
        help="Repeat skill-name=references,scripts,assets",
    )
    parser.add_argument("--with-mcp", action="store_true", help="Create a valid empty mcp.json")
    args = parser.parse_args()

    plugin_name = args.name.strip()
    description = args.description.strip()
    if not PLUGIN_NAME_RE.fullmatch(plugin_name) or len(plugin_name) > 64:
        return fail("plugin name must satisfy Agent Plugins 1.0.0 naming rules")
    if not description:
        return fail("description is required")

    try:
        skills = [assignment(raw, "--skill") for raw in args.skill]
        resource_assignments = [assignment(raw, "--skill-resources") for raw in args.skill_resources]
    except argparse.ArgumentTypeError as exc:
        return fail(str(exc))

    skill_descriptions: dict[str, str] = {}
    for name, skill_description in skills:
        if len(name) > 64 or not SKILL_NAME_RE.fullmatch(name):
            return fail(f"invalid Skill name: {name}")
        if len(skill_description) > 1024:
            return fail(f"Skill description exceeds 1024 characters: {name}")
        if name in skill_descriptions:
            return fail(f"duplicate Skill name: {name}")
        skill_descriptions[name] = skill_description

    skill_resources: dict[str, list[str]] = {}
    for name, raw_resources in resource_assignments:
        if name not in skill_descriptions:
            return fail(f"--skill-resources targets undeclared Skill: {name}")
        resources = [item.strip() for item in raw_resources.split(",") if item.strip()]
        unknown = sorted(set(resources) - RESOURCE_DIRS)
        if unknown:
            return fail(f"unsupported resources for {name}: {', '.join(unknown)}")
        skill_resources[name] = list(dict.fromkeys(resources))

    target = Path(args.path).expanduser().resolve() / plugin_name
    if target.exists() or target.is_symlink():
        return fail(f"target already exists: {target}")

    try:
        target.mkdir(parents=True, mode=0o700)
        manifest = {
            "$schema": PLUGIN_SCHEMA,
            "name": plugin_name,
            "version": "0.1.0",
            "description": description,
        }
        (target / "plugin.json").write_text(
            json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
        for skill_name, skill_description in skill_descriptions.items():
            skill_root = target / "skills" / skill_name
            skill_root.mkdir(parents=True, mode=0o700)
            title = " ".join(part.capitalize() for part in skill_name.split("-"))
            body = (
                "---\n"
                f"name: {skill_name}\n"
                f"description: {json.dumps(skill_description, ensure_ascii=False)}\n"
                "---\n\n"
                f"# {title}\n\n"
                f"{skill_description}\n\n"
                "## Workflow\n\n"
                "Replace this starter with the real tool sequence, resource routes, safety boundaries, and acceptance criteria.\n"
            )
            (skill_root / "SKILL.md").write_text(body, encoding="utf-8")
            for resource in skill_resources.get(skill_name, []):
                (skill_root / resource).mkdir(mode=0o700)
        if args.with_mcp:
            (target / "mcp.json").write_text(
                json.dumps({"$schema": MCP_SCHEMA, "mcpServers": {}}, indent=2) + "\n",
                encoding="utf-8",
            )
    except Exception as exc:  # pragma: no cover - defensive CLI boundary
        return fail(f"create candidate: {exc}")

    print(
        json.dumps(
            {
                "ok": True,
                "plugin_root": str(target),
                "skills": sorted(skill_descriptions),
                "mcp_json_created": args.with_mcp,
                "next": "fill or remove scaffold resources, configure components, then run validate_plugin.py",
            },
            ensure_ascii=False,
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
