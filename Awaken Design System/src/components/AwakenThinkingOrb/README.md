# AwakenThinkingOrb

UI-only 的 Canvas 思考球组件。它通过 MIT 许可的 `thinking-orbs` 包接入六种独立调校的视觉状态，不读取 Desktop、runtime 或线程状态。

## 使用边界

- `AwakenThinkingOrb` 是 Design System 对官方 `ThinkingOrb` 的窄 wrapper，统一提供 Awaken 命名与 `aw-thinking-orb` class，不复制第三方 Canvas 引擎。
- `AwakenThinkingOrbStatus` 以透明行内布局组合思考球、`思考中` 文案和三点节奏动效；不提供背景、描边、圆角胶囊或固定宽度。内部使用 `state="composing"`、`size={64}`、`speed={2.4}`。
- 支持 `working`、`searching`、`solving`、`listening`、`composing`、`shaping` 六种状态。
- 官方没有第七种 `thinking` state；官网的 `Thinking....` 只是 `composing` 动效的展示文案。
- 只支持官方分别调校的 `64` 和 `20` 两档尺寸；它们不是同一 Canvas 的比例缩放。
- `speed` 只是各状态内置速度的倍率。Gallery 中 `composing` 使用本轮选定的 `2.4` 倍速，其余状态保留默认倍率。
- Gallery 独立观测六种 Canvas 状态和 Thinking 行内状态；Desktop 仅在 `AwakenThoughtTrace` 的 active 状态消费 20px 行内规格，完成或错误后由 ThoughtTrace 切换为 disclosure，64px 仅用于独立展示。run 生命周期、等待首个输出和完成状态仍全部由 Desktop 决定，组件不得自行推断。

## 可访问性与性能

- 底层以 `<canvas role="img">` 渲染，并允许 consumer 通过 `aria-label` 提供语义。
- 状态行使用 `role="status"` 承载完整语义，三点动效与内部 Canvas 对辅助技术隐藏，避免重复播报。
- `theme="auto"` 跟随系统明暗主题，也可由 consumer 显式传入 `dark` 或 `light`；状态文字和 Canvas 必须使用同一主题。
- 状态文案使用轻量扫光，并在 `prefers-reduced-motion` 下停止动画。
- 六态 Gallery 舞台使用局部 `data-theme="dark"` 复现官方深色参考；这不会改变组件在其他 consumer 中的自动主题行为。
- 底层在离屏、标签页隐藏或 `prefers-reduced-motion` 生效时暂停或回退为静态帧。

## 来源

- 官网：https://orbs.jakubantalik.com/
- 源码：https://github.com/Jakubantalik/thinking-orbs
- 依赖：`thinking-orbs@^0.1.1`
