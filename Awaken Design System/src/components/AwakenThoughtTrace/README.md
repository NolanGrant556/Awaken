# AwakenThoughtTrace

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenThoughtTrace` 用于在执行流中折叠展示单段模型思考轨迹，支持进行中、完成和错误状态，也可以带持续时间标签。它适合跟随用户提示、工具调用和回答一起出现在运行叙事中；父级负责编排顺序、传入文本与状态，本组件只负责 `details/summary` 折叠容器和状态样式。

## 文件

- `AwakenThoughtTrace.tsx` - 组件实现。
- `AwakenThoughtTrace.css` - 组件局部样式入口。
- `AwakenThoughtTrace.states.ts` - 可观测状态与模拟 props。
- `AwakenThoughtTrace.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
