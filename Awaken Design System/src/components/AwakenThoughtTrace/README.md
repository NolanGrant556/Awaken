# AwakenThoughtTrace

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenThoughtTrace` 用于在执行流中展示并折叠单段模型思考轨迹，支持进行中、完成和错误状态，也可以带持续时间标签。进行中以 `AwakenThinkingOrbStatus` 的 20px 行内规格作为 summary，显示思考球、`Thinking` 文字和三点节奏动效，不使用胶囊表面；完成或错误后移除仅属于运行状态的粒子球，改用 20px 对齐盒内的 12px Chevron 表达可展开的历史入口。状态文字使用 12px 层级，reasoning 正文始终从状态文字起点对齐。父级负责编排顺序、传入文本、状态和当前主题，本组件不读取 runtime。

Awaken Desktop 在 run 尚未产生首个可见输出时传入空文本和 `active`，因此只显示动态 Thinking 状态行；收到 runtime 明确提供的 reasoning summary 后，文本在同一组件下方流式展开。`done` 默认显示带 Chevron 的 `Thought` 历史入口，展开时 Chevron 向下、折叠时向右；最终回答出现后仍由 Desktop 的“执行过程”外层 disclosure 收起。`error`、自定义 `label` 和 `durationLabel` 属于通用组件 API。

## 文件

- `AwakenThoughtTrace.tsx` - 组件实现。
- `AwakenThoughtTrace.css` - 组件局部样式入口。
- `AwakenThoughtTrace.states.ts` - 可观测状态与模拟 props。
- `AwakenThoughtTrace.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- `theme` 由 consumer 显式传入时必须同时作用于状态文字和 Canvas；`auto` 才跟随系统主题。
- 公共 API 变化时同步更新可观测状态。
