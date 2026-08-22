# AwakenReconnectStatus

纯 UI 的供应商重连状态行，用于 Workbench turn body 当前执行游标。组件不读取 runtime、SSE、Electron 或 thread，只消费标题、去敏细节和受控/非受控展开状态。

- `title`：当前可见状态，例如“正在等待网络”或“正在重新连接 2/5”。
- `detail` / `details`：可选的去敏英文短句；组件去空、去重后放入展开面板。
- `open` / `onOpenChange`：受控展开；未传 `open` 时使用 `defaultOpen`。
- 没有细节时渲染只读状态行；有细节时使用带 `aria-expanded` / `aria-controls` 的按钮。
- 图标只通过 `AwakenIcon concept="network"` 获取，颜色、字号、间距和 reduced-motion 使用 Design System 语义 token。

业务层负责决定 live-only identity、重试次数、清理时机和去敏内容。本组件不得持久化状态、推断 provider 错误或建立第二套重连策略。

验证：

```bash
npm --workspace @awaken/design-system run typecheck
npm --workspace @awaken/design-system run smoke:render
```
