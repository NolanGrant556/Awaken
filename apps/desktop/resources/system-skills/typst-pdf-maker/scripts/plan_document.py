#!/usr/bin/env python3
"""根据内容清单生成可复核、不可注入命令的 Typst 构建计划。"""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import sys
from copy import deepcopy
from pathlib import Path
from typing import Any


SKILL_ROOT = Path(__file__).resolve().parent.parent
DEFAULT_CATALOG = SKILL_ROOT / "references" / "routing-catalog.json"
DEFAULT_OUTPUT = Path(".typst-build-plan.json")
PDF_OPERATION_SIGNALS = {
    "pdf-read",
    "pdf-merge",
    "pdf-split",
    "pdf-rotate",
    "pdf-form",
    "pdf-ocr",
    "pdf-watermark",
    "pdf-encrypt",
    "pdf-decrypt",
    "pdf-extract",
}
SLIDE_ENGINES = {"diatypst", "polylux", "touying"}


def load_object(path: Path) -> dict[str, Any]:
    try:
        parsed = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise ValueError(f"JSON 文件不存在：{path}") from exc
    except json.JSONDecodeError as exc:
        raise ValueError(f"JSON 格式错误：{path}: {exc}") from exc
    if not isinstance(parsed, dict):
        raise ValueError(f"JSON 顶层必须是对象：{path}")
    return parsed


def string_set(value: Any, field: str) -> set[str]:
    if value is None:
        return set()
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        raise ValueError(f"{field} 必须是字符串数组")
    return {item.strip().lower() for item in value if item.strip()}


def validate_manifest(manifest: dict[str, Any]) -> dict[str, Any]:
    task_kind = manifest.get("task_kind", "new-document")
    output = manifest.get("output", "pdf")
    features = manifest.get("content_features", {})
    if not isinstance(task_kind, str) or not task_kind.strip():
        raise ValueError("manifest.task_kind 必须是非空字符串")
    if not isinstance(output, str) or not output.strip():
        raise ValueError("manifest.output 必须是非空字符串")
    if not isinstance(features, dict):
        raise ValueError("manifest.content_features 必须是对象")
    normalized = deepcopy(manifest)
    normalized["task_kind"] = task_kind.strip().lower()
    normalized["output"] = output.strip().lower()
    normalized["signals"] = sorted(string_set(manifest.get("signals"), "manifest.signals"))
    normalized["hard_constraints"] = sorted(
        string_set(manifest.get("hard_constraints"), "manifest.hard_constraints")
    )
    normalized["content_features"] = features
    return normalized


def validate_catalog(catalog: dict[str, Any]) -> None:
    expected = {"builtins": 2, "templates": 20, "packages": 43}
    ids: list[str] = []
    for field, count in expected.items():
        entries = catalog.get(field)
        if not isinstance(entries, list) or len(entries) != count:
            actual = len(entries) if isinstance(entries, list) else "缺失"
            raise ValueError(f"catalog.{field} 必须有 {count} 项，当前为 {actual}")
        for entry in entries:
            if not isinstance(entry, dict) or not isinstance(entry.get("id"), str):
                raise ValueError(f"catalog.{field} 包含非法条目")
            ids.append(entry["id"])
            if field != "builtins" and not isinstance(entry.get("version"), str):
                raise ValueError(f"catalog.{field}.{entry['id']} 缺少固定版本")
    duplicates = sorted({identifier for identifier in ids if ids.count(identifier) > 1})
    if duplicates:
        raise ValueError("catalog 存在重复 ID：" + ", ".join(duplicates))


def typst_version(binary: Path) -> str:
    if not binary.is_absolute() or not binary.is_file():
        raise ValueError("--typst-bin 必须指向存在的绝对文件")
    try:
        completed = subprocess.run(
            [str(binary), "--version"],
            check=True,
            capture_output=True,
            text=True,
            timeout=15,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise ValueError(f"无法执行 Typst 版本检查：{exc}") from exc
    output = (completed.stdout or completed.stderr).strip()
    if not output:
        raise ValueError("Typst 没有返回版本信息")
    return output.splitlines()[0]


def fingerprint(manifest: dict[str, Any], catalog_version: str, version: str) -> str:
    encoded = json.dumps(
        {"manifest": manifest, "catalog_version": catalog_version, "typst_version": version},
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("utf-8")
    return "sha256:" + hashlib.sha256(encoded).hexdigest()


def universe_ref(entry: dict[str, Any]) -> str:
    return f"@preview/{entry['id']}:{entry['version']}"


def public_item(
    entry: dict[str, Any],
    *,
    kind: str | None = None,
    reason: str,
    auto_apply: bool,
) -> dict[str, Any]:
    item: dict[str, Any] = {
        "id": entry["id"],
        "kind": kind or entry.get("kind", "universe-package"),
        "reason": reason,
        "adoption": entry.get("adoption"),
        "ownership": entry.get("ownership"),
        "conflict_group": entry.get("conflict_group"),
        "status": entry.get("status", "ready"),
        "auto_apply": auto_apply,
    }
    for field in ("path", "adoption", "fidelity_warning", "patch_note", "block_reason"):
        if entry.get(field) is not None:
            item[field] = entry[field]
    if entry.get("version"):
        item["version"] = entry["version"]
        item["universe_ref"] = universe_ref(entry)
    return item


def new_plan(fp: str, catalog_version: str, version: str) -> dict[str, Any]:
    return {
        "schema_version": 1,
        "fingerprint": fp,
        "catalog_version": catalog_version,
        "typst_version": version,
        "reused": False,
        "channel": "typst-pdf",
        "state": "ready",
        "base": None,
        "style_owner": None,
        "enhancements": [],
        "optional_idea": None,
        "confirmation_candidates": [],
        "exclusions": [],
        "question": None,
        "next_action": "minimum-compile-preflight",
    }


def match(entries: list[dict[str, Any]], signals: set[str], field: str) -> list[dict[str, Any]]:
    matches: list[dict[str, Any]] = []
    for entry in entries:
        negative = set(entry.get("negative_signals", []))
        if signals & negative:
            continue
        if signals & set(entry.get(field, [])):
            matches.append(entry)
    return matches


def redirect(plan: dict[str, Any], channel: str, reason: str) -> dict[str, Any]:
    plan.update(
        channel=channel,
        state="redirect",
        reason=reason,
        next_action="exit-typst-skill",
    )
    return plan


def choose_base(
    manifest: dict[str, Any],
    catalog: dict[str, Any],
    plan: dict[str, Any],
) -> dict[str, Any]:
    task_kind = manifest["task_kind"]
    output = manifest["output"]
    signals = set(manifest["signals"])

    if task_kind == "pdf-operation" or signals & PDF_OPERATION_SIGNALS:
        return redirect(plan, "awaken-pdf", "该任务属于 PDF 阅读或加工，应使用 Awaken pdf Skill。")
    if task_kind == "presentation" or output in {"ppt", "pptx", "slides"}:
        return redirect(plan, "awaken-ppt", "普通演示文稿应使用 Awaken ppt Skill 生成可编辑 PPTX。")

    explicit_typst_slides = task_kind == "typst-presentation" or bool(
        signals & (SLIDE_ENGINES | {"typst-presentation", "pdf-slides"})
    )
    if explicit_typst_slides:
        candidates = [
            entry
            for entry in catalog["templates"] + catalog["packages"]
            if entry["id"] in SLIDE_ENGINES and entry["id"] in signals
        ]
        if len(candidates) != 1:
            plan.update(
                state="ambiguous",
                question="请选择一个 Typst 幻灯片引擎：Diatypst、Polylux 或 Touying。",
                next_action="ask-one-question",
            )
            return plan
        entry = candidates[0]
        kind = "universe-template" if entry in catalog["templates"] else "universe-package"
        selected = public_item(
            entry,
            kind=kind,
            reason="用户明确要求 Typst/PDF 幻灯片并指定了引擎。",
            auto_apply=False,
        )
        if kind == "universe-template":
            selected["init_argv"] = ["init", selected["universe_ref"]]
            selected["init_destination"] = {
                "append": "absolute-workspace-path",
                "must_be_within_workspace": True,
            }
        plan["base"] = selected
        plan["style_owner"] = entry["id"]
        return plan

    if task_kind == "existing-typst":
        plan["base"] = {
            "id": "existing-project",
            "kind": "existing-project",
            "ownership": "global",
            "reason": "保留用户现有 Typst 项目及其样式所有权。",
            "auto_apply": True,
            "status": "ready",
        }
        plan["style_owner"] = "existing-project"
        return plan

    builtins = catalog["builtins"]
    if task_kind == "markdown-to-pdf" or signals & {"markdown-source", "markdown-to-pdf"}:
        entry = next(item for item in builtins if item["id"] == "markdown-entry")
        plan["base"] = public_item(
            entry,
            reason="已有 Markdown 来源，使用 Skill 自带适配器。",
            auto_apply=True,
        )
        plan["style_owner"] = entry["id"]
        return plan

    templates = catalog["templates"]
    candidates = match(templates, signals, "hard_signals")
    reason = "用户明确指定模板或出版格式。"
    if not candidates and signals & {"resume", "cv"}:
        plan.update(
            state="ambiguous",
            question="这是 ATS 简历、学术 CV，还是视觉作品集简历？",
            next_action="ask-one-question",
        )
        return plan
    if not candidates:
        candidates = match(templates, signals, "soft_signals")
        reason = "文档结构信号唯一匹配该模板。"
    if len(candidates) > 1:
        plan.update(
            state="ambiguous",
            question="多个文档模板同时匹配："
            + "、".join(entry["id"] for entry in candidates)
            + "。请确认优先格式。",
            next_action="ask-one-question",
        )
        return plan
    if len(candidates) == 1:
        entry = candidates[0]
        selected = public_item(
            entry,
            kind="universe-template",
            reason=reason,
            auto_apply=False,
        )
        selected["init_argv"] = ["init", selected["universe_ref"]]
        selected["init_destination"] = {
            "append": "absolute-workspace-path",
            "must_be_within_workspace": True,
        }
        plan["base"] = selected
        plan["style_owner"] = entry["id"]
        if entry.get("status") == "blocked":
            plan.update(state="blocked", next_action="stop-before-init")
        elif entry.get("status") == "patch-required":
            plan.update(state="patch-required", next_action="apply-patch-then-preflight")
        return plan

    entry = next(item for item in builtins if item["id"] == "report-entry")
    plan["base"] = public_item(
        entry,
        reason="未指定专用模板，使用 Skill 自带专业报告基底。",
        auto_apply=True,
    )
    plan["style_owner"] = entry["id"]
    return plan


def trigger_matches(features: dict[str, Any], triggers: dict[str, Any]) -> bool:
    for key, wanted in triggers.items():
        actual = features.get(key)
        if isinstance(wanted, bool):
            if actual is not wanted:
                return False
        elif isinstance(wanted, (int, float)):
            if not isinstance(actual, (int, float)) or actual < wanted:
                return False
        elif actual != wanted:
            return False
    return True


def choose_packages(
    manifest: dict[str, Any],
    catalog: dict[str, Any],
    plan: dict[str, Any],
) -> dict[str, Any]:
    if plan["state"] in {"redirect", "ambiguous", "blocked"}:
        return plan
    if (plan.get("base") or {}).get("conflict_group") == "slides-engine":
        return plan

    features = manifest["content_features"]
    signals = set(manifest["signals"])
    constraints = set(manifest["hard_constraints"])
    required: list[dict[str, Any]] = []
    low_risk: list[dict[str, Any]] = []
    confirmation: list[dict[str, Any]] = []
    exclusions: list[dict[str, str]] = []

    for entry in catalog["packages"]:
        if entry.get("conflict_group") == "slides-engine":
            continue
        if not trigger_matches(features, entry.get("triggers", {})):
            continue
        blocked_by = sorted((signals | constraints) & set(entry.get("negative_signals", [])))
        if blocked_by:
            exclusions.append({"id": entry["id"], "reason": "被以下约束排除：" + "、".join(blocked_by)})
            continue
        item = public_item(
            entry,
            reason="内容特征满足 " + json.dumps(entry.get("triggers", {}), ensure_ascii=False, sort_keys=True),
            auto_apply=entry["adoption"] in {"A", "B"},
        )
        if entry["adoption"] == "A":
            required.append(item)
        elif entry["adoption"] == "B":
            low_risk.append(item)
        else:
            item["auto_apply"] = False
            confirmation.append(item)

    grouped: dict[str, list[dict[str, Any]]] = {}
    for item in required + low_risk + confirmation:
        group = item.get("conflict_group")
        if group:
            grouped.setdefault(group, []).append(item)
    for group, items in grouped.items():
        if len(items) < 2:
            continue
        hard = [item for item in items if item.get("adoption") == "A"]
        if len(hard) == 1:
            winner = hard[0]
            low_risk = [item for item in low_risk if item.get("conflict_group") != group]
            confirmation = [item for item in confirmation if item.get("conflict_group") != group]
            for item in items:
                if item["id"] != winner["id"]:
                    exclusions.append(
                        {"id": item["id"], "reason": f"冲突组 {group} 已由 {winner['id']} 占用。"}
                    )
            continue
        plan.update(
            state="ambiguous",
            question=f"请为能力组 {group} 选择唯一实现：" + "、".join(item["id"] for item in items),
            next_action="ask-one-question",
            enhancements=required,
            exclusions=exclusions,
        )
        return plan

    plan["enhancements"] = required
    plan["exclusions"] = exclusions
    if low_risk:
        plan["optional_idea"] = low_risk[0]
        for item in low_risk[1:]:
            plan["exclusions"].append(
                {"id": item["id"], "reason": "未请求的 B 类增强最多自动采用一个。"}
            )
    plan["confirmation_candidates"] = confirmation
    if confirmation:
        plan.update(
            state="needs-confirmation",
            question="是否采用以下结构或样式增强：" + "、".join(item["id"] for item in confirmation),
            next_action="ask-one-question",
        )
    return plan


def create_plan(
    manifest: dict[str, Any],
    catalog: dict[str, Any],
    version: str,
    previous: dict[str, Any] | None,
) -> dict[str, Any]:
    catalog_version = str(catalog.get("catalog_version", "unknown"))
    fp = fingerprint(manifest, catalog_version, version)
    plan = new_plan(fp, catalog_version, version)
    plan = choose_packages(manifest, catalog, choose_base(manifest, catalog, plan))
    # 旧计划只作为“是否复用决策”的提示；完整计划始终从可信清单重新生成，
    # 不会把工作区中的自由文本命令复制回新计划。
    if previous and previous.get("fingerprint") == fp:
        plan["reused"] = True
        if plan["state"] == "ready":
            plan["next_action"] = "reuse-recomputed-plan"
    return plan


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="生成 Awaken Typst PDF 构建计划")
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--typst-bin", type=Path, required=True)
    parser.add_argument("--catalog", type=Path, default=DEFAULT_CATALOG)
    parser.add_argument("--previous-plan", type=Path)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    try:
        manifest = validate_manifest(load_object(args.manifest))
        catalog = load_object(args.catalog)
        validate_catalog(catalog)
        version = typst_version(args.typst_bin)
        previous_path = args.previous_plan
        if previous_path is None and args.output.is_file():
            previous_path = args.output
        previous = load_object(previous_path) if previous_path and previous_path.is_file() else None
        plan = create_plan(manifest, catalog, version, previous)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        temporary = args.output.with_name(args.output.name + ".tmp")
        temporary.write_text(
            json.dumps(plan, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
        temporary.replace(args.output)
    except (OSError, ValueError) as exc:
        print(f"错误：{exc}", file=sys.stderr)
        return 2
    print(json.dumps({"plan": str(args.output), "state": plan["state"]}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
