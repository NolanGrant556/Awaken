# AwakenThoughtTrace

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenThoughtTrace` 展示一次思考的标题与正文。运行中使用“思考 中”和文字扫光，不在执行过程使用思考球。标题为 15px/24px、贴近文字的行盒；“思考”为次要文字色，后面的“片刻 / 12 秒 / 中”为可读的 subtle 色，中间留空格。完成且无可靠秒数时显示“思考 片刻”，有可靠秒数时显示“思考 12 秒”。正文使用 15px / 24px 并沿同一左边缘排列。

无正文时只有不可展开的状态行。已有正文默认收起，`open/onOpenChange` 支持与工具一致的受控披露；用户展开后新正文原位增长，状态变化不覆盖选择。长正文在主滚动内展开，末尾收起按钮派发 `aw-trace-collapse`，由 Desktop 保持阅读位置和焦点。没有真实耗时就不传 `durationLabel`。

展开收起高度为 180ms，透明度为 120ms。折叠内容按需挂载，关闭过渡结束后卸载；历史一次展示全部已有正文。减少动态效果时直接切换，终态停止扫光动画。`theme` 保留 API 兼容，实际颜色来自所属主题容器，不再绑定 Canvas。

## 文件

- `AwakenThoughtTrace.tsx` - 组件实现。
- `AwakenThoughtTrace.css` - 组件局部样式入口。
- `AwakenThoughtTrace.states.ts` - 可观测状态与模拟 props。
- `AwakenThoughtTrace.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
