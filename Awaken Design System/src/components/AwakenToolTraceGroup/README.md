# AwakenToolTraceGroup

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及“工具轨迹”组件族的 Gallery 入口。

## 用途

`AwakenToolTraceGroup` 用于承载 Desktop 投影出的 Activity Batch，并把已展平的 action 渲染为 `AwakenToolTraceLine`，不再嵌套第二层工具组。组标题是内容宽度的单行 disclosure：文字左对齐、主 Chevron 常驻；活动中默认展开，完成或历史重放默认收起，用户手动选择优先。收起的活动组由标题承担 1.6 秒文字扫光；展开后改由当前活动 Tool 扫光。`label/detail` 允许 Desktop 传入确定性摘要，`open/onOpenChange` 支持按稳定 Batch ID 受控；未传时保留自动状态行为。

展开后的子项与组标题、详情卡共用原有内容列，不额外缩进。实时执行的子项默认以 80ms 递进进入；回看已完成历史时传入 `animateItems=false`，使子项同帧完整显示。父级负责排序、分段、摘要和选择状态，本组件只负责 disclosure、轨迹列表与回调代理。

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
