# Slice 18: 最终收口与真实验收 Spec

## Why

Slice 1-17 已经把 Desktop 主体验迁到 Run Workbench，并补齐 narrative、inspector、composer、组件库和高精度 runtime events。Slice 18 负责最后收口：确认旧 Dashboard / 卡片墙 / 开发者日志不再留在产品主路径，更新 Desktop 交付文档，并用真实 LLM 与视觉检查证明当前交付不是 mock 半成品。

## What Changes

- 检查并删除仍在 `apps/desktop` 或 `packages/ui` 产品主路径中的旧 Dashboard/card wall/dev log 入口。
- 保留仍被 Run Workbench 或 playground 使用的 UI-only 组件，不为了关键词清理误删设计系统资产。
- 更新 Desktop README / handoff docs，描述 Run Workbench 当前主界面、真实验收命令和已知边界。
- 执行真实 LLM smoke：普通问答、只读项目探索、approval approve/reject 至少一路、cancel 或 reconnect。
- 执行视觉 QA，确认桌面 IA 是左 Sidebar、中 Run Narrative、右 Inspector、底 Composer。

## Impact

- 主要触及 docs 和少量旧入口清理。
- 不重构 runtime/tool/approval 策略。
- 不删除 playground-only 或仍被 design system docs 引用的组件，除非确认不再使用。
- 不 commit/push。

## Requirements

### Requirement: Old main UI is gone from product path

#### Scenario: Search old UI terms

- **WHEN** running `rg -n "Dashboard|card wall|developer log|开发者日志|卡片墙" apps/desktop packages/ui`
- **THEN** no product main-path match remains
- **AND** any remaining documentation-only references are outside that acceptance command

### Requirement: Documentation reflects Run Workbench

#### Scenario: Developer reads Desktop docs

- **WHEN** reading Desktop README or handoff docs
- **THEN** the primary UI is described as Run Workbench
- **AND** smoke / QA commands are current
- **AND** known non-goals such as cloud sync and marketplace remain explicit

### Requirement: Real LLM acceptance

#### Scenario: Real model smoke suite runs

- **WHEN** configured LLM credentials are present
- **THEN** plain Q&A smoke passes
- **AND** read-only project exploration smoke passes
- **AND** approval approve/reject at least one path passes
- **AND** cancel or reconnect smoke passes

### Requirement: Visual QA

#### Scenario: Desktop is inspected visually

- **WHEN** the built app or dev renderer is opened
- **THEN** first viewport shows Run Workbench IA: left Sidebar, center Run Narrative, right Inspector, bottom Composer
- **AND** no Dashboard/card wall/developer log panel is visible
- **AND** text does not visibly overlap in the main workbench shell

### Requirement: No mock acceptance

#### Scenario: Final validation summary is prepared

- **WHEN** reporting completion
- **THEN** mock/playground checks are separated from real LLM smoke
- **AND** no mock run is presented as real acceptance

## Modified Requirements

- Documentation now treats Run Workbench as the Desktop primary product surface, not an experimental replacement.

## Removed Requirements

- None.
