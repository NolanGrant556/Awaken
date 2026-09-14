#!/usr/bin/env python3
"""Validate the project-specific provider reference structure."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


SKILL_ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = SKILL_ROOT.parents[2]
PROVIDERS_ROOT = SKILL_ROOT / "references" / "providers"
MAIN_CATALOG = REPO_ROOT / "apps" / "desktop" / "src" / "main" / "model-profile-store.ts"
AGENT_METADATA = SKILL_ROOT / "agents" / "openai.yaml"
TOP_LEVEL_REFERENCES = (
    "references/Awaken功能与调用面.md",
    "references/协议与能力判定.md",
    "references/用量查询合同.md",
)
EXPECTED_PROVIDERS = {
    "anthropic",
    "custom",
    "deepseek",
    "glm",
    "google-gemini",
    "kimi",
    "minimax",
    "openai",
    "qwen",
    "volcengine-doubao",
}
EXPECTED_PROVIDER_IDS = {
    "anthropic": {"anthropic"},
    "custom": {"custom"},
    "deepseek": {"deepseek"},
    "glm": {"glm", "glm-coding-plan"},
    "google-gemini": {"google"},
    "kimi": {"kimi-cn", "kimi-coding-plan"},
    "minimax": {"minimax", "minimax-token-plan"},
    "openai": {"openai"},
    "qwen": {"qwen", "qwen-token-plan"},
    "volcengine-doubao": {"doubao", "volcengine", "volcengine-agent-plan"},
}
REQUIRED_HEADINGS = (
    "## 官方资料入口",
    "## Awaken 当前接入",
    "## 三协议兼容矩阵",
    "## 鉴权、端点与模型",
    "## 上下文、多模态与工具",
    "## 提示词缓存",
    "## 推理开启关闭",
    "## 推理强度",
    "## 用量、余额、限流与套餐",
    "### 用量查询合同",
    "## Awaken 落地规则",
    "## 更新检查清单",
)
DATE_RE = re.compile(r"官方资料复核日期：\d{4}-\d{2}-\d{2}")
HTTPS_RE = re.compile(r"https://[^)\s>]+")
MODEL_UPDATE_REFERENCE = "../../协议与能力判定.md#9-更新证据优先级"
CATALOG_START = "export const BUILTIN_MODEL_PROVIDERS"
PROVIDER_ID_RE = re.compile(r'^\s*id:\s*"([^"]+)"', re.MULTILINE)


def builtin_provider_array() -> str:
    if not MAIN_CATALOG.is_file():
        return ""
    source = MAIN_CATALOG.read_text(encoding="utf-8")
    if CATALOG_START not in source:
        return ""
    return source.split(CATALOG_START, 1)[1].split("\n];", 1)[0]


def builtin_provider_ids() -> set[str]:
    return set(PROVIDER_ID_RE.findall(builtin_provider_array()))


def builtin_default_models() -> dict[str, str]:
    provider_array = builtin_provider_array()
    matches = list(PROVIDER_ID_RE.finditer(provider_array))
    defaults: dict[str, str] = {}
    for index, match in enumerate(matches):
        end = matches[index + 1].start() if index + 1 < len(matches) else len(provider_array)
        block = provider_array[match.end():end]
        default = re.search(r'^\s*default_model:\s*"([^"]*)"', block, re.MULTILINE)
        if default:
            defaults[match.group(1)] = default.group(1)
    return defaults


def validate_provider(provider: str) -> list[str]:
    errors: list[str] = []
    directory = PROVIDERS_ROOT / provider
    document = directory / "供应商资料.md"
    if not directory.is_dir():
        return [f"缺少供应商目录：{provider}"]
    unexpected = sorted(path.name for path in directory.iterdir() if path.name != document.name)
    if unexpected:
        errors.append(f"{provider}: 目录含非约定文件：{', '.join(unexpected)}")
    if not document.is_file():
        return errors + [f"{provider}: 缺少 供应商资料.md"]

    text = document.read_text(encoding="utf-8")
    if f"]({MODEL_UPDATE_REFERENCE})" not in text:
        errors.append(f"{provider}: 缺少模型资料更新规则引用")
    policy_file, _ = MODEL_UPDATE_REFERENCE.split("#", 1)
    if not (directory / policy_file).is_file():
        errors.append(f"{provider}: 模型资料更新规则文件不存在")
    if not DATE_RE.search(text):
        errors.append(f"{provider}: 缺少 YYYY-MM-DD 官方资料复核日期")
    for heading in REQUIRED_HEADINGS:
        if heading not in text:
            errors.append(f"{provider}: 缺少标题 {heading}")
    if provider != "custom" and not HTTPS_RE.search(text):
        errors.append(f"{provider}: 缺少 HTTPS 官方链接")
    if "| Chat Completions |" not in text or "| Responses |" not in text or "| Messages |" not in text:
        errors.append(f"{provider}: 三协议矩阵行不完整")
    for provider_id in sorted(EXPECTED_PROVIDER_IDS[provider]):
        if f"`{provider_id}`" not in text:
            errors.append(f"{provider}: 缺少 Awaken provider ID `{provider_id}`")
    if "## 用量、余额、限流与套餐" in text and "## Awaken 落地规则" in text:
        usage_section = text.split("## 用量、余额、限流与套餐", 1)[1].split(
            "## Awaken 落地规则", 1
        )[0]
        for marker in ("凭据", "Awaken 当前"):
            if marker not in usage_section:
                errors.append(f"{provider}: 用量查询合同缺少 `{marker}`")
    return errors


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="校验供应商资料结构；默认模型直接从代码读取，不要求在文档中重复列出。")
    parser.add_argument("--list-defaults", action="store_true", help="校验后输出当前代码默认模型；不是官方最新模型查询")
    args = parser.parse_args(argv)
    errors: list[str] = []
    if not PROVIDERS_ROOT.is_dir():
        errors.append(f"缺少目录：{PROVIDERS_ROOT}")
    else:
        actual = {path.name for path in PROVIDERS_ROOT.iterdir() if path.is_dir()}
        for provider in sorted(EXPECTED_PROVIDERS - actual):
            errors.append(f"缺少供应商目录：{provider}")
        for provider in sorted(actual - EXPECTED_PROVIDERS):
            errors.append(f"未登记供应商目录：{provider}")
        for provider in sorted(EXPECTED_PROVIDERS):
            errors.extend(validate_provider(provider))

    documented_provider_ids = set().union(*EXPECTED_PROVIDER_IDS.values())
    catalog_provider_ids = builtin_provider_ids()
    if not catalog_provider_ids:
        errors.append(f"无法从 Desktop catalog 读取 provider ID：{MAIN_CATALOG}")
    else:
        for provider_id in sorted(catalog_provider_ids - documented_provider_ids):
            errors.append(f"Desktop catalog provider ID 未映射到供应商资料：`{provider_id}`")
        for provider_id in sorted(documented_provider_ids - catalog_provider_ids):
            errors.append(f"供应商资料 provider ID 不在 Desktop catalog：`{provider_id}`")

    skill_file = SKILL_ROOT / "SKILL.md"
    skill_text = skill_file.read_text(encoding="utf-8")
    if not skill_text.startswith("---\nname: llm-provider-management\n"):
        errors.append("SKILL.md frontmatter name 缺失或不匹配")
    for reference in TOP_LEVEL_REFERENCES:
        if not (SKILL_ROOT / reference).is_file():
            errors.append(f"缺少顶层 reference：{reference}")
        if reference not in skill_text:
            errors.append(f"SKILL.md 未直接链接顶层 reference：{reference}")
    usage_contract = SKILL_ROOT / "references" / "用量查询合同.md"
    if usage_contract.is_file():
        usage_text = usage_contract.read_text(encoding="utf-8")
        for provider_id in sorted(documented_provider_ids):
            if f"`{provider_id}`" not in usage_text:
                errors.append(f"用量查询合同缺少 provider ID `{provider_id}`")
    for provider in sorted(EXPECTED_PROVIDERS):
        target = f"references/providers/{provider}/供应商资料.md"
        if target not in skill_text:
            errors.append(f"SKILL.md 未直接链接：{target}")

    if not AGENT_METADATA.is_file():
        errors.append("缺少 agents/openai.yaml")
    else:
        agent_text = AGENT_METADATA.read_text(encoding="utf-8")
        if '$llm-provider-management' not in agent_text:
            errors.append("agents/openai.yaml default_prompt 未引用 $llm-provider-management")
        if not re.search(r'^\s*display_name:\s*"[^"]+"\s*$', agent_text, re.MULTILINE):
            errors.append("agents/openai.yaml 缺少带引号的 display_name")
        short_description = re.search(
            r'^\s*short_description:\s*"([^"]+)"\s*$', agent_text, re.MULTILINE
        )
        if not short_description or not 25 <= len(short_description.group(1)) <= 64:
            errors.append("agents/openai.yaml short_description 必须为 25–64 个字符")

    if errors:
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print(f"OK: {len(EXPECTED_PROVIDERS)} 个供应商目录和资料结构有效")
    if args.list_defaults:
        print("当前 checkout 的内置默认模型（官方可用性与能力须查对应官方文档）：")
        for provider_id, default_model in sorted(builtin_default_models().items()):
            print(f"- {provider_id}: {default_model or '由用户填写'}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
