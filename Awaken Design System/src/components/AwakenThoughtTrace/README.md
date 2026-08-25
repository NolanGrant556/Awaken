# AwakenThoughtTrace

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenThoughtTrace` 用于在执行流中展示并折叠单段模型思考轨迹，支持进行中、完成和错误状态，也可以带持续时间标签。进行中以 `AwakenThinkingOrbStatus` 的 20px 行内规格作为 summary，显示思考球、`思考中` 文字和三点节奏动效，不使用胶囊表面；完成或错误后移除仅属于运行状态的粒子球，立即显示位于文字右侧的 12px Chevron 与静态历史标题，避免自动收尾时出现孤立 Chevron。标题触发区按内容宽度收紧并始终透明，默认使用统一浅色文字与 Chevron，悬停或键盘聚焦时只提亮前景色；正文从同一左边缘向下展开，不增加连接线或子级缩进，并只使用 `grid-template-rows` 裁切高度，不通过更快的透明度动画制造仍占布局的空白。父级负责编排顺序、传入文本、状态和当前主题，本组件不读取 runtime。

Awaken Desktop 在 run 尚未产生首个可见输出时传入空文本和 `active`，因此只显示动态思考中状态行；收到 runtime 明确提供的 reasoning summary 后，文本在同一组件下方流式展开。`active` 自动展开，`done` 默认折叠成带 Chevron 的 `思考过程` 历史入口，用户点击后的选择优先于自动状态；最终回答出现后仍由 Desktop 的“执行过程”外层 disclosure 收起。折叠的历史正文不进入 DOM；打开时挂载完整文本，关闭动画结束后卸载，数据仍由 props 完整保留。`animateEntry=false` 关闭正文的挂载入场动画，供历史 replay 首帧直接显示；它不停止 active 状态本身的思考球。`error`、自定义 `label`、`durationLabel` 和 `defaultOpen` 属于通用组件 API。

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
