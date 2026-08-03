# AwakenToolTraceLine

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenToolTraceLine` 用于在执行流中展示单次 Tool、MCP、Skill 或终端调用。摘要固定为一行，只保留图标、自然语言标题、必要短上下文和语义状态。Chevron 始终保留 16px 布局宽度，但默认透明，仅在行 hover、`focus-visible` 或已展开时可见，因此不会产生横向跳动。运行中的标题使用 1.6 秒从左到右的文字扫光；完成、异常、取消和中断立即停止，`prefers-reduced-motion` 下保持静态高亮。单条轨迹默认收起，点击或键盘 Enter/Space 后从原有左边缘展开详情，不增加连接线或层级缩进。

新路径优先传入 `detailCard`：卡片由只用于可访问名称的 `title`、可复制的 `copyText` 和 `sections[]` 组成，section 支持 `text | code | json | diff` 与 `default | muted | command | danger | success` 语义 tone；展开后直接显示正文，不重复绘制“Shell 详情/工具详情”标题。卡片使用主题化灰色 code surface，命令、普通终端输出与成功/错误分别使用可辨识的终端语义颜色。卡片最高 320px，正文区是唯一的内部滚动 owner，保留可见滚动条、键盘焦点和文本选择；右上角只显示无底卡的复制图标，并与滚动条、首段正文分别预留空间。`output/outputLabel` 继续作为旧历史兼容回退。`open/onOpenChange` 可由 Desktop 按稳定 Tool ID 控制，未传时保留组件本地状态。

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
