# AwakenSendButton

此目录包含一个独立的 Awaken 设计系统生产组件和可复用状态；它不再重复占用 Gallery 顶层入口。

## 用途

`AwakenSendButton` 用于 composer 的发送/停止入口，根据 `ready`、`loading` 和 `mode` 展示发送、发送中或停止运行。父级负责判断输入是否可发送、处理点击和运行取消；组件只负责圆形图标按钮、tooltip、禁用态和加载指示。

## 文件

- `AwakenSendButton.tsx` - 组件实现。
- `AwakenSendButton.css` - 组件局部样式入口。
- `AwakenSendButton.states.ts` - 可观测状态与模拟 props。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
- Gallery 在“输入栏”组件族的完整 Composer 中观测可发送、空输入、发送中、停止运行和禁用状态；原 `awaken-send-button` URL 跳转到该组件族。
