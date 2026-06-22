# AwakenOriginalThinking

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenOriginalThinking` 是对 Original Thinking 数学曲线 Loader 的 React 复刻。默认渲染刻意保留原始公式、粒子数量、尾迹长度、旋转周期、脉冲周期、标题、标签、公式文本和 `View All` 外链，适合在组件库中作为“原始思考”动效参考使用。

## 文件

- `AwakenOriginalThinking.tsx` - 组件实现。
- `AwakenOriginalThinking.css` - 组件局部样式入口。
- `AwakenOriginalThinking.states.ts` - 可观测状态与模拟 props。
- `AwakenOriginalThinking.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 该组件是视觉复刻件，不要在没有新参考的情况下调整公式常量、动画时长、粒子数或默认排版。
- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 动画只在组件挂载期间运行，卸载时必须取消 `requestAnimationFrame`。
