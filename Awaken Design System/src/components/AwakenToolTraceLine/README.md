# AwakenToolTraceLine

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenToolTraceLine` 用于在执行流中展示单次 Tool、MCP、Skill 或终端调用。摘要固定为一行，标题旁只放紧凑上下文；有 `output` 时图标、标题、状态和最右侧 Chevron 组成内容宽度的 disclosure，同行剩余空白不响应 hover 或点击。可折叠行始终透明，默认使用统一浅色前景，悬停或键盘聚焦时只提亮文字、普通图标与 Chevron，不显示灰色圆角卡片底；运行中和错误等语义状态色继续保留。运行中自动展开，结束后默认折叠，点击后从轨迹原有左边缘向下展示独立输出卡片，不增加细连接线或子级缩进。`kind` 决定来源标识和完成态图标，`outputTruncated` 只负责提示边界，不改变原始数据。`animateEntry` 默认为 `true`，用于实时新增轨迹的轻量入场；终态“执行过程”这类重新挂载已完成历史的 consumer 必须传入 `false`，避免把已有记录伪装成逐条到达。

父级负责工具数据、选择目标、bounded output 和 Inspector 行为；组件只负责轨迹呈现。传入 `onSelect` 时，点击行会在切换输出的同时通知父级。

## 文件

- `AwakenToolTraceLine.tsx` - 组件实现。
- `AwakenToolTraceLine.css` - 组件局部样式入口。
- `AwakenToolTraceLine.states.ts` - 可观测状态与模拟 props。
- `AwakenToolTraceLine.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
