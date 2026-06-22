# AwakenToast

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenToast` 用于在窗口右下角显示短暂的非阻塞提示，例如工作区选择失败、操作已触发或运行状态提醒。父级负责维护 `visible` 状态、消息内容和关闭回调，本组件只负责固定位置、无障碍 `status` 语义和定时触发 `onClose`，不负责通知队列或业务错误处理。

## 文件

- `AwakenToast.tsx` - 组件实现。
- `AwakenToast.css` - 组件局部样式入口。
- `AwakenToast.states.ts` - 可观测状态与模拟 props。
- `AwakenToast.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
