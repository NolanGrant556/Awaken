# Slice 10: 设计令牌 v2

## 目标

把 Run Workbench 已验证出来的视觉语言沉淀到 `@awaken/ui` token，减少新组件和 Desktop Workbench 样式中的散落颜色、阴影、半径常量。

## 范围

- 升级 `packages/ui/src/styles/tokens.css`，新增 Workbench / Narrative / Inspector / Terminal / Diff 语义 token。
- 迁移新 Run Workbench 相关 UI 组件到语义 token。
- 迁移 Desktop Workbench 新样式中的主要散落颜色、半径、阴影。
- 更新设计系统文档，明确 `docs/component-library.md` 是历史档案，v2 token 来源是 Run Workbench。

## 验收标准

- 新 Workbench 组件不依赖散落硬编码颜色值。
- 新 Workbench 组件不依赖散落 shadow/radius magic number。
- token 命名表达语义，不表达具体页面位置。
- `packages/ui` build/typecheck 通过。
- 文档明确 v2 token 来源是 Run Workbench，不再是旧 HTML demo。

## 暂不做

- 不做多主题系统。
- 不做用户自定义主题。
- 不做 dark mode。
