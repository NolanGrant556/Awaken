# AwakenContextUsagePanel

纯 UI 的上下文用量面板。父级传入七项 typed 分类、可选“其他/协议开销”、当前总输入用量、输入窗口和关闭回调；组件不读取 runtime、Electron、线程或模型 profile，也不消费 compact diagnostics 或 invalidation reason。首行标题与关闭按钮同行；汇总行展示「N% 当前使用」与「当前总输入 / 可用输入窗口 上下文用量」，其下为分类进度条与明细。进度条和图例颜色由分类 `id` 绑定；0 值分类可以不绘制 segment，但不能让后续分类顺位改色。存在缓存或成本信号时，分类明细后直接按父级传入的 nullable usage 展示缓存、输入、命中/未命中、输出与成本，不添加“上次请求”标题、压缩诊断区块或失效原因；缺失字段不折算为 0。只有 Context Epoch 边界原因而没有缓存/成本合同的记录不显示这些明细，避免把 Host 的 `initial_baseline` 等内部状态冒充供应商缓存事实。

- `AwakenContextUsagePanel.tsx`：结构、格式化和局部初始焦点。
- `AwakenContextUsagePanel.css`：基于现有 Design System token 的亮暗主题与窄窗口内部滚动。
- `AwakenContextUsagePanel.states.ts`：分类与旧记录降级状态。
- `AwakenContextUsagePanel.demo.tsx`：独立渲染夹具；面向人工观测的可点击场景由 `AwakenComposerBar.demo.tsx` 统一展示。
