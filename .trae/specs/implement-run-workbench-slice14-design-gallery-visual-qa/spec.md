# Slice 14: Design Gallery 与视觉验收 Spec

## Why

Run Workbench 的设计系统组件已经抽象到 `@awaken/ui`，但开发者还需要一个不用启动完整 runtime 的设计验收台，集中检查 Workbench idle/running/approval/done/error、narrative item、Inspector tabs、Composer busy/done 等状态。Slice 14 补强 playground 与轻量验收脚本，确保 mock 只留在 playground。

## What Changes

- 更新 `packages/ui/playground` 的 Run Workbench gallery。
- 增加 Run Workbench mock data，覆盖 idle、running、approval、done、error。
- Gallery 展示所有 narrative item 类型：prompt、planning、thinking、exploring、tool、approval、fileChange、answer。
- Gallery 展示 Inspector tabs 与 composer busy/done 状态。
- 增加组件级 render smoke 脚本。
- 使用 browser/CDP 做截图级 DOM 与布局验收。

## Impact

- 只影响 `packages/ui/playground`、UI smoke 脚本和必要的 package script。
- mock 数据只存在 playground，不进入 runtime reducer、Desktop runtime 或 Go runtime。
- 不引入 Storybook，不改变产品入口。

## Requirements

### Requirement: Workbench gallery states

#### Scenario: Developer switches mock state

- **WHEN** 开发者打开 Run Workbench playground
- **THEN** 可以切换 idle、running、approval、done、error mock

### Requirement: Narrative coverage

#### Scenario: Developer reviews narrative item types

- **WHEN** gallery 渲染完整 narrative sample
- **THEN** prompt、planning、thinking、exploring、tool、approval、fileChange、answer 都可见

### Requirement: Inspector and composer coverage

#### Scenario: Developer reviews peripheral states

- **WHEN** gallery 切换 mock state
- **THEN** Inspector tabs 可见，Composer 可展示 busy 与 done/follow-up 状态

### Requirement: Render smoke

#### Scenario: CI or developer runs UI render smoke

- **WHEN** 运行 UI render smoke
- **THEN** 核心 workbench/narrative/inspector/composer 组件能完成 server render

### Requirement: Visual QA

#### Scenario: Browser QA probes gallery

- **WHEN** 在 browser 中检查关键状态
- **THEN** 不出现文本重叠、按钮文字溢出、嵌套卡片感或横向 body overflow

## Modified Requirements

- Playground 的 Run Workbench tab 从局部预览升级为 Design Gallery。

## Removed Requirements

- 无。
