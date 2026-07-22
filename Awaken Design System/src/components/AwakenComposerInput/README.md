# AwakenComposerInput

此目录包含输入栏内部使用的 Awaken 设计系统输入组件和可观测演示状态。

## 用途

`AwakenComposerInput` 用于输入任务草稿，是受控 textarea，支持自动高度、禁用态、Enter 提交回调和 Shift+Enter 换行。它适合被 `AwakenComposerBar` 包裹使用；父级负责 value、onChange、onSubmit、校验和发送按钮，本组件只处理文本输入、键盘提交和高度约束。

## 文件

- `AwakenComposerInput.tsx` - 组件实现。
- `AwakenComposerInput.css` - 组件局部样式入口。
- `AwakenComposerInput.states.ts` - 可观测状态与模拟 props。
- Gallery 不再为它提供独立原子组件入口；输入状态统一在 `AwakenComposerBar.demo.tsx` 的“输入栏”组件族中观测，原 `awaken-composer-input` URL 会跳转到该入口。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
