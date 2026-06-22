# AwakenPlatformProvider

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenPlatformProvider` 用于把当前平台写入 React context，让下游组件可以统一读取 `platform`、`isMac`、`isWindows` 和快捷键修饰键。它适合包裹设计系统演示、桌面壳或局部快捷键区域；父级负责检测或选择平台，子级负责消费 context 并呈现对应快捷键，不在 provider 内做系统探测。

## 文件

- `AwakenPlatformProvider.tsx` - 组件实现。
- `AwakenPlatformProvider.css` - 组件局部样式入口。
- `AwakenPlatformProvider.states.ts` - 可观测状态与模拟 props。
- `AwakenPlatformProvider.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
