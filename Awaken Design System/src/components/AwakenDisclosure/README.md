# AwakenDisclosure

此目录包含一个独立的 Awaken 设计系统组件及其可观测状态。Gallery 将它集中放在“工具轨迹”组件族中展示，不再单独占用目录入口。

## 用途

`AwakenDisclosure` 用于把补充信息收纳在可展开标题下，例如执行日志、工具详情、解释文本或可选设置。父级负责提供标题、元信息、内容以及是否受控的 open 状态；组件只负责折叠交互、可访问属性和内容显隐，不解析或重排内部内容。

## 文件

- `AwakenDisclosure.tsx` - 组件实现。
- `AwakenDisclosure.css` - 组件局部样式入口。
- `AwakenDisclosure.states.ts` - 可观测状态与模拟 props。
- `AwakenDisclosure.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
- 原 `awaken-disclosure` URL 跳转到 `awaken-tool-trace`，用于观察 Desktop 中“执行过程”收纳工具轨迹的真实组合。
