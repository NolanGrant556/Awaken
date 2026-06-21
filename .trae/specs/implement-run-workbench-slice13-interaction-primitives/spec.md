# Slice 13: 重复交互抽象 Spec

## Why

Run Workbench 已有 Sidebar、Run Narrative、Inspector、Composer 的主路径，但 tab、tooltip、copy、collapse、auto-scroll 等重复交互仍散落在 Desktop 与局部组件中。Slice 13 将这些交互沉淀到 UI-only 组件或 hook，让 Desktop 保持只负责状态与回调。

## What Changes

- `@awaken/ui` 收敛 Inspector tabs 的键盘切换行为。
- `@awaken/ui` 新增 `Tooltip`。
- `@awaken/ui` 新增 `Disclosure`。
- `@awaken/ui` 新增 `CopyButton`。
- `@awaken/ui` 新增 `useAutoScrollTail`。
- Desktop Run Workbench 使用新的 tooltip、copy、disclosure、auto-scroll hook。
- Playground 增加重复交互预览。

## Impact

- `packages/ui` 继续保持 UI-only，不引入 runtime、Electron、AG-UI、A2UI、SSE 或 `window.awaken`。
- Desktop 的 Inspector tab、tool detail、copy command/path、feed tail-follow 行为更一致。
- 不改变 Go runtime、tool、approval、artifact 或 LLM 调用策略。

## Requirements

### Requirement: UI-only interaction primitives

#### Scenario: Components stay presentation-only

- **WHEN** `Tooltip`、`Disclosure`、`CopyButton`、`useAutoScrollTail` 被实现
- **THEN** 它们只依赖 React、浏览器 DOM 能力、既有 UI tokens 与 controlled props/callbacks

### Requirement: Keyboard tabs

#### Scenario: User navigates Inspector tabs by keyboard

- **WHEN** 用户 focus 在 Inspector tabs 上并按 ArrowLeft/ArrowRight/Home/End
- **THEN** tab focus 和 active tab 按可用 tab 顺序切换

### Requirement: Icon tooltip labels

#### Scenario: User hovers or focuses an icon-only button

- **WHEN** 用户 hover 或 focus 有 tooltip 的 icon-only button
- **THEN** 用户看到轻量 tooltip，按钮仍有 `aria-label` 或 `title`

### Requirement: Disclosure semantics

#### Scenario: User expands tool details

- **WHEN** 用户点击 disclosure trigger
- **THEN** 内容展开或收起，trigger 暴露 `aria-expanded`

### Requirement: Copy feedback

#### Scenario: User copies command or path

- **WHEN** 用户点击 copy button
- **THEN** clipboard 写入目标文本并展示短暂成功反馈

### Requirement: Auto-scroll respects reading

#### Scenario: User scrolls up during streaming

- **WHEN** feed 正在追加 narrative items 且用户停留在底部
- **THEN** feed 自动跟随底部
- **WHEN** 用户向上滚动阅读历史
- **THEN** 新内容追加时不强制把用户拉回底部

## Modified Requirements

- Inspector tabs 从点击-only 升级为支持键盘切换。
- Run stage auto-scroll 从 Desktop 局部 ref 逻辑收敛为 UI hook。

## Removed Requirements

- 无。
