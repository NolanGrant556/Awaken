# AwakenToolTraceLine

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenToolTraceLine` 展示一次工具、MCP、Skill、Hook 或命令调用。文字为 15px/24px，行盒贴近内容，与正文共用左边缘。标题拆成动词（secondary 色）和事实（subtle 色），中间留空格。固定工具图标列已取消，`icon` 仅保留类型兼容。活动使用文字扫光；失败、取消、中断和待处理不在标题旁另标文字。Chevron 在 hover、focus-visible 和展开时可见，布局宽度始终保留。标题只表示这次调用，不展示工具结果或状态标签；结果和 diff 放进默认收起的详情卡。详情卡没有顶栏空白、复制按钮或执行环境 JSON。

详情默认收起，支持 `open/onOpenChange` 受控状态。`detailCard.sections` 按 text/code/json/diff 展示：JSON 参数与诊断进一步折叠，命令保留有界输出和复制，diff 显示文件标题、旧新行号及安全的轻量 token 着色，每文件前 80 行之外可展开全部已保留内容。截断标记由 consumer 提供，不能把截断内容显示成完整结果。卡片正文最高 320px，滚动不穿透主过程。

`fileLabel/onOpenFile` 产生独立文件按钮，详情箭头是另一个按钮，不嵌套交互目标。文件校验、磁盘读取和源码定位由 Desktop/Main 执行；组件不读取文件系统。`onCopyDetail` 仍保留在 API 中但不渲染复制入口；旧 `output/outputLabel` 继续兼容。

展开与收起为 180ms，透明度为 120ms。关闭过渡结束后卸载内容，减少动态效果时直接切换。历史不交错重播。

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
