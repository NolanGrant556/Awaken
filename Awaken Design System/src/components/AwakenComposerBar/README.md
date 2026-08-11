# AwakenComposerBar

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及集中展示输入与已提交提示的 Gallery 组件族入口。

## 用途

`AwakenComposerBar` 用于提供紧凑任务输入栏，可作为中心入口、底部 dock 输入栏或表单内嵌 prompt 编辑器使用，并展示 repo/model/status 等元信息。它适合运行中的追加输入、首屏快速开始、停靠 composer 和需要复用 Slash/token 编辑语义的受控表单；父级负责受控输入、结构化 token、metaItems、提交/停止和添加上下文动作，本组件负责选择原生 `AwakenComposerInput` 或统一 plaintext token editor，并布局由 `AwakenIconButton` 实现的 `AwakenContextButton`、`AwakenSendButton` 和两者的 `AwakenTooltip`。带 `progress` 的上下文用量入口渲染为圆环占比指示，仅指针 hover 用 `AwakenTooltip` 展示「N% 预计使用」与父级传入的 `value`；打开面板期间与关闭后焦点回到入口时不展示 tip，指针离开后再 hover 才出现。点击仍由父级打开 `AwakenContextUsagePanel`，组件本身不拥有用量数据或面板内容。

`variant="embedded"` 只渲染输入 surface，不渲染上下文与发送工具栏；`submitOnEnter={false}` 保留普通多行编辑，`inputAriaLabel` 由宿主表单提供准确名称。内嵌形态仍复用同一 `slashPalette`、结构化 token 和 `onStructuredInputChange`，不建立第二套 Slash UI 或 token DOM。

当前 Desktop 的 `center` 与 `dock` 形态共享同一个受控模型选择入口：父级通过 `metaItems` 传入当前模型、可选模型、供应商 Logo、选中回调和“添加模型”动作，组件负责列表的展开、收起、选中标记及键盘关闭；支持推理强度的模型进入二级面板后继续复用同一枚 Logo，并用一条可拖动、可键盘操作的 Codex 式离散轨道选择档位。轨道不显示 `Effort`、提交按钮或档位文字；最低档直接落在轨道左端且不重复画圆点，其余档位圆点始终保留在中性轨道层，最高档提示点在右端内侧留出间距，而选中最高档后的白色滑块仍到达轨道右端。彩色光流从上层覆盖已经过的点，白色圆形滑块再覆盖当前点，只有滑块右侧尚未选择的中性灰点露出。往回拖时光流裁切边界后退，被覆盖的档位点会自然重新出现，不通过隐藏节点模拟。光流直接复用 `AwakenMemoryLightFlow` 的紧凑绘制变体，并单独增强细丝线宽、亮度和局部辉光；普通档为绿色，最右 `max` 档为红色，轨道本身不保存模型配置。拖动期间滑块与填充边界精确跟随指针，松手、取消或键盘选择后收敛到所选档位；`max` 仍只在指针进入最右区域并跨过像素回差后触发，避免提前变红。点击、拖动或 Arrow/Home/End 只能落在 capability 真实存在的档位并立即回调；单纯悬停或移动不变更档位。Gallery 提供三档、两档和无档位 mock，但不读取或保存真实模型配置。

模型菜单打开时，底部模型触发按钮使用同一套 240ms 横向伸展动效：右端位置保持稳定，胶囊从当前内容宽度向左拉长到菜单宽度，按钮内容随胶囊居中；模型列表卡片继续使用原有尺寸和定位，不参与伸展。`center` 首页与 `dock` 对话页共享同一实现，只在点击时读取一次按钮当前宽度，不新增计时状态、运行时订阅或持续动画循环；系统启用 `prefers-reduced-motion` 时直接显示展开后的按钮。

`center` 还可以通过受控 `projectPicker` 在输入 surface 下方显示项目作用域低栏。当前作用域直接从 `options[].selected` 派生；`label` 控制菜单与可访问名称，`options[].icon === null` 表示不渲染图标，用于无项目时的“选择项目”占位状态；项目选项未传 `icon` 时继续使用默认文件夹图标。已选项目且父级传入 `clearAction` 时，文件夹图标会在整段触发器 hover 或清除按钮获得键盘焦点后原位换成圆形 `X`；点击 `X` 只回调清除动作，不打开项目菜单。菜单只展示父级传入的选项并回调选择动作，「创建项目」也只回调父级。组件不读取 workspace、FileStore 或 Electron，也不保存第二份作用域选择；`dock` 不渲染这条首页低栏。Gallery 的 `center-idle` 状态提供可切换 mock 项目、取消选择和创建入口。

无结构化 token 时继续使用原生 textarea。结构化 token 存在时，整个输入流只有一个 `contenteditable="plaintext-only"` editing host；每个 token 是带稳定 `data-composer-token-id` 的 `contenteditable="false"` 原子节点，token 前文、token 间正文和最后尾文都可在同一光标、选区与 IME 上下文中编辑。长前缀自然换行后，剩余宽度允许时 token、尾文与光标继续当前行；token 的视觉间距不改写正文值。

组件以 DOM 为编辑期事实源：`input` 后把 token 前的文本映射为对应 `leadingText`，把最后一个 token 后的文本映射为 `value`，再通过 `onStructuredInputChange` 交给父级持久化。React 不控制 editing host 的 children；只有 token identity/label 或外部 draft 真正变化时才在 layout 阶段重建 DOM，并按不包含 token label 的逻辑 offset 恢复选区。composition 期间禁止重建，复制纯文本时也不把 token label 写入正文或剪贴板。

## 文件

- `AwakenComposerBar.tsx` - 组件实现，包括模型二级面板、档位圆点、滑块和光流裁切。
- `AwakenComposerInlineInput.tsx` - 带结构化 token 时的单一 editing host 与 DOM ↔ structured state adapter。
- `AwakenComposerBar.css` - 组件局部样式入口。
- `AwakenComposerBar.states.ts` - 可观测状态与模拟 props。
- `AwakenComposerBar.demo.tsx` - “输入栏”组件族 Gallery 演示；先用真实 Workbench 缩略场景区分上方滚动输出与底部停靠输入栏，并通过真实元信息入口打开 `AwakenContextUsagePanel`，再集中观测 ComposerBar、底层 ComposerInput、添加上下文图标按钮、发送/停止按钮、工具提示和已提交提示状态。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- `projectPicker` 只属于 `center` 首页视觉；无项目/项目 authority、项目排序、选择清理和创建弹窗均留在 Desktop。
- token label/source/path 不进入 `value`；selection、复制、持久化和提交均只使用结构化 segment。
- 不使用 zero-width caret sentinel；空 segment 由真实空文本节点表示。
- 公共 API 变化时同步更新可观测状态。
- 原 `awaken-composer-input`、`awaken-context-button`、`awaken-user-prompt-block`、`awaken-composer-dock`、`awaken-icon-button`、`awaken-send-button` 与 `awaken-tooltip` URL 均跳转到“输入栏”组件族；这些小组件继续保留生产源码与公开导出。
