# AwakenSidebarThreadSpinner

左侧栏项目树中的 UI-only 线程运行指示器。它集中在“「左侧栏」组件库” Gallery 入口中查看，不单独占用组件目录入口。

## 用途

`AwakenSidebarThreadSpinner` 只渲染侧栏线程行末尾的 14px 方形点阵运行指示。它组合固定的 `AwakenDotmSquare3`，保持 2px 方点和 1px 间距。Desktop 根据线程是否存在 `running` 或 `cancelling` run 决定是否显示；组件本身不读取 runtime 状态，也不推断线程生命周期。

## 文件

- `AwakenSidebarThreadSpinner.tsx` - 组件实现与可访问名称。
- `AwakenSidebarThreadSpinner.css` - 固定行内几何、颜色和指针行为；点阵动效由 `AwakenDotmSquare3` 拥有。
- `AwakenSidebarThreadSpinner.states.ts` - running / cancelling 可观测状态。
- `AwakenSidebarThreadSpinner.demo.tsx` - 独立视觉 Demo，由“「左侧栏」组件库”集中承载入口。
- `index.ts` - 组件导出。

## 规则

- 只接收展示文案，不读取线程或 run。
- 默认可访问名称为“加载中”。
- `prefers-reduced-motion` 下由 `AwakenDotmSquare3` 回退为静态明暗点阵，不改变 14px 行内几何。
