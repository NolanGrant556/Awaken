# AwakenComposerBar

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及集中展示输入与已提交提示的 Gallery 组件族入口。

## 用途

`AwakenComposerBar` 用于提供紧凑任务输入栏，可作为中心入口或底部 dock 输入栏使用，并展示 repo/model/status 等元信息。它适合运行中的追加输入、首屏快速开始和停靠 composer；父级负责受控输入、结构化 token、metaItems、提交/停止和添加上下文动作，本组件负责选择原生 `AwakenComposerInput` 或统一 plaintext token editor，并布局 `AwakenContextButton` 与发送按钮。

当前 Desktop 的 `center` 与 `dock` 形态共享同一个受控模型选择入口：父级通过 `metaItems` 传入当前模型、可选模型、选中回调和“添加模型”动作，组件负责列表的展开、收起、选中标记及键盘关闭。Gallery 使用 mock profile 复现这条交互，但不读取或保存真实模型配置。

无结构化 token 时继续使用原生 textarea。结构化 token 存在时，整个输入流只有一个 `contenteditable="plaintext-only"` editing host；每个 token 是带稳定 `data-composer-token-id` 的 `contenteditable="false"` 原子节点，token 前文、token 间正文和最后尾文都可在同一光标、选区与 IME 上下文中编辑。长前缀自然换行后，剩余宽度允许时 token、尾文与光标继续当前行；token 的视觉间距不改写正文值。

组件以 DOM 为编辑期事实源：`input` 后把 token 前的文本映射为对应 `leadingText`，把最后一个 token 后的文本映射为 `value`，再通过 `onStructuredInputChange` 交给父级持久化。React 不控制 editing host 的 children；只有 token identity/label 或外部 draft 真正变化时才在 layout 阶段重建 DOM，并按不包含 token label 的逻辑 offset 恢复选区。composition 期间禁止重建，复制纯文本时也不把 token label 写入正文或剪贴板。

## 文件

- `AwakenComposerBar.tsx` - 组件实现。
- `AwakenComposerInlineInput.tsx` - 带结构化 token 时的单一 editing host 与 DOM ↔ structured state adapter。
- `AwakenComposerBar.css` - 组件局部样式入口。
- `AwakenComposerBar.states.ts` - 可观测状态与模拟 props。
- `AwakenComposerBar.demo.tsx` - “输入栏”组件族 Gallery 演示；先用真实 Workbench 缩略场景标出 `AwakenComposerDock` 的底部停靠边界，再观测 ComposerBar 状态和 `AwakenUserPromptBlock` 的已提交提示状态。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- token label/source/path 不进入 `value`；selection、复制、持久化和提交均只使用结构化 segment。
- 不使用 zero-width caret sentinel；空 segment 由真实空文本节点表示。
- 公共 API 变化时同步更新可观测状态。
