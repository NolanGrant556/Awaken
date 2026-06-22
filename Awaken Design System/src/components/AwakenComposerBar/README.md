# AwakenComposerBar

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenComposerBar` 用于提供紧凑任务输入栏，可作为中心入口或底部 dock 输入栏使用，并展示 repo/model/status 等元信息。它适合运行中的追加输入、首屏快速开始和停靠 composer；父级负责受控输入、metaItems、提交/停止和添加上下文动作，本组件负责布局 `AwakenComposerInput`、`AwakenContextButton` 与发送按钮。

## 文件

- `AwakenComposerBar.tsx` - 组件实现。
- `AwakenComposerBar.css` - 组件局部样式入口。
- `AwakenComposerBar.states.ts` - 可观测状态与模拟 props。
- `AwakenComposerBar.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
