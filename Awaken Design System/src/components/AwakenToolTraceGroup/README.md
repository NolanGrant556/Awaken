# AwakenToolTraceGroup

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及“工具轨迹”组件族的 Gallery 入口。

## 用途

`AwakenToolTraceGroup` 用于把同一探索阶段或同一叙事节点下的多条工具轨迹组合在一起，并把每条 action 渲染为 `AwakenToolTraceLine`。组本身也是内容宽度的单行 disclosure：文字左对齐、Chevron 位于最右侧，触发器始终透明，默认使用统一浅色前景，悬停或键盘聚焦时只提亮前景色；任一 action 运行时自动展开，全部结束后默认折叠为“数量 + 结果”的摘要；展开后的子项从组原有左边缘出现，不再额外缩进。实时执行的子项默认以 80ms 递进进入；外层仅在回看已完成历史时传入 `animateItems=false`，使子项同帧完整显示。父级负责排序、分组语义和选中后的 Inspector 行为，本组件只负责组摘要、轨迹列表和 `onSelectAction` 代理。

Gallery 的“工具轨迹”组件族同时展示 Desktop 在最终回答后用 `AwakenDisclosure` 收纳整段“执行过程”的真实组合关系。`AwakenDisclosure`、`AwakenToolTraceGroup` 和 `AwakenToolTraceLine` 继续保持独立源码与公开导出；原 `awaken-disclosure`、`awaken-tool-trace-group`、`awaken-tool-trace-line` URL 均跳转到统一入口。

## 文件

- `AwakenToolTraceGroup.tsx` - 组件实现。
- `AwakenToolTraceGroup.css` - 组件局部样式入口。
- `AwakenToolTraceGroup.states.ts` - 可观测状态与模拟 props。
- `AwakenToolTraceGroup.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
