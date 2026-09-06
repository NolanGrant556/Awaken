# AwakenUserPromptBlock

此目录包含一个独立的 Awaken 设计系统组件和可观测演示状态；Gallery 在“输入栏 `AwakenComposerBar`”组件族入口中集中展示这些状态。

## 用途

`AwakenUserPromptBlock` 用于在执行流中展示用户发起本轮任务的原始提示文本。它适合出现在运行叙事的开头，帮助用户回看当前 run 的目标；父级负责保存提示、决定顺序和是否显示，本组件只负责提示内容的块状展示。

长提示默认收起到 3 行等价高度，折叠溢出在第三行字形下沿裁切，并用约 6px 的气泡底色短渐变贴着下沿淡出，底部内边距为 0，不用省略号；单击气泡后展开到 9 行，超过部分在气泡内部滚动。拖选正文、token 标签或附件文件名不会触发展开。展开后点击气泡外空白处会复原。可改写会话在气泡下方常驻撤销、编辑、复制透明图标按钮；没有撤销/编辑回调时，复制入口仍在展开后出现。复制按钮不使用底卡或阴影。

结构化 token 与前后正文在同一个文本流中投影。组件保留 `leadingText` 的原始空格和换行，并让“前文 → token → 后文”按气泡实际宽度自然换行；token 不拥有独立 flex 行，后文也不会被拆成单独文本块。Prompt bubble 与全部 Composer 形态共用 `14/20px` 正文、`16px` 线性图标、`5px` 图标/label 间距、`8px` token 后间距和 medium 字重；token 通过几何中心对齐中英文、英文与数字，不再维护消息卡片专用的字号、图标或下沉偏移。正文、token 标签（Skill/文件/capability/Plugin）与附件显示名可局部选择；`data-aw-select-all-root` 承接 Desktop 的 `Cmd/Ctrl+A`，组件监听逻辑选区并在其穿过 Token 时给整个原子节点补齐选中底色，因此 SVG 图标、间距和 label 之间不会留下白洞。默认复制与选区复制都按可见正文流拼装（含 token 标签），不把软换行写成硬换行，也不包含 Skill 路径或隐藏引导。父级传入 `copyText` 时可覆盖工具栏默认值。

## 文件

- `AwakenUserPromptBlock.tsx` - 组件实现。
- `AwakenUserPromptBlock.css` - 组件局部样式入口。
- `AwakenUserPromptBlock.states.ts` - 可观测状态与模拟 props。
- `AwakenUserPromptBlock.demo.tsx` - 供 Gallery bundle 渲染冒烟使用；顶层观测入口由 `AwakenComposerBar.demo.tsx` 统一承载。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
