# Slice 11: Workbench Layout 组件

## 目标

把 Run Workbench 的布局骨架从 Desktop `App.tsx` 中抽到 `@awaken/ui`，让 Desktop 只组合 UI-only 组件并传入数据、回调和内容。

## 范围

- 新增 `WorkbenchShell`、`WorkbenchSidebarFrame`、`RunStage`、`ComposerDock`。
- 复用已存在的 UI-only `InspectorRail`，不重复实现。
- `packages/ui` 布局组件不 import runtime、AG-UI、A2UI、Electron、SSE 或 `window.awaken`。
- Desktop 替换本地 workbench grid/run stage/composer/inspector wrapper。
- Playground 增加 workbench mock layout 场景，覆盖 idle/running/approval/done。

## 验收标准

- Desktop App 不再手写主要 workbench grid/stage/composer/inspector wrapper。
- `packages/ui` 布局组件只接收 props/children/callbacks。
- 1280px 宽度下 Workbench 三栏稳定展示。
- 较窄窗口下 Inspector 不遮挡 composer。
- playground 可展示 idle、running、approval、done 状态。

## 暂不做

- 不做移动端网页适配。
- 不做可拖拽面板宽度。
