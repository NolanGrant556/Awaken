# AwakenDotmSquare3

UI-only 的固定 5×5 方形点阵加载指示器。组件拥有点阵几何、向内螺旋明暗动画和 reduced-motion 回退，不读取 Desktop 或 runtime 状态。

## 使用边界

- `AwakenDotmSquare3` 是整个 5×5 矩阵组件；单个点只是内部渲染单元，不对外导出。
- 组件只开放 `size`、`dotSize`、`ariaLabel` 和 `className`；不再提供其他图案、点形、速度、颜色或发光变体。
- Gallery 默认以 36px 容器和 5px 方点观测；侧栏线程指示由 `AwakenSidebarThreadSpinner` 组合为 14px 容器和 2px 方点。Desktop 仍只决定是否显示。
- 组件根节点使用行内 `span` 语义，可安全组合在项目树按钮内。

## 可访问性与动画

- consumer 通过 `ariaLabel` 传入当前语义。
- 点单元使用 `aria-hidden="true"`，整个矩阵作为一个 `status` 暴露。
- `prefers-reduced-motion` 生效时停止循环动画，保留静态明暗反馈。
