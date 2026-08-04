# AwakenContextUsagePanel

纯 UI 的上下文用量面板。父级传入六项可求和分类、输入窗口和关闭回调；组件不读取 runtime、Electron、线程或模型 profile。首行标题与关闭按钮同行；汇总行展示「N% 已使用」与「~当前 / 可用 Token」，其下为分类进度条与明细。进度条和图例颜色由分类 `id` 绑定；0 值分类可以不绘制 segment，但不能让后续分类顺位改色。

- `AwakenContextUsagePanel.tsx`：结构、格式化和局部初始焦点。
- `AwakenContextUsagePanel.css`：基于现有 Design System token 的亮暗主题与窄窗口内部滚动。
- `AwakenContextUsagePanel.states.ts`：分类与旧记录降级状态。
- `AwakenContextUsagePanel.demo.tsx`：独立渲染夹具；面向人工观测的可点击场景由 `AwakenComposerBar.demo.tsx` 统一展示。
