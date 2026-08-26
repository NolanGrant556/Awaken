# AwakenSkills

Skills 管理页的 UI-only 组合组件。Gallery 中只注册“「技能」组件库”这一处入口：页面先展示完整组合，再集中展示各个独立小组件。每个公开小组件仍拥有自己的目录、API、states、Demo 和 README，但不单独占用 Gallery 侧栏入口。

## 组件

- `AwakenSkills`：受控的四视图组合面板，只拥有来源切换、搜索投影和组合布局。
- `AwakenSkillCard`：独立目录中的统一 Skill 卡片；卡片主体触发预览，右侧开关独立触发启停。
- `AwakenSwitch`：原子目录中的受控 switch；Skill 卡片通过兼容外壳消费同一视觉实现。
- `AwakenFolder`：原子目录中的项目入口，项目窗口打开期间通过 `open` 保持展开。
- `AwakenProjectSkillsDialog`：独立目录中的项目 Skills 窗口，组合统一卡片列表。
- `AwakenSkillPreviewDialog`：独立目录中的只读 Skill 包阅读窗口，展示包内文件树、原始 YAML frontmatter 与安全 Markdown 正文。

## 受控状态

`AwakenSkills` 不读取 catalog 或本地文件，也不自行持久化状态。Desktop 通过 props 提供当前视图、搜索值、各来源数据和已打开项目，通过以下 callbacks 接管真实行为：

- `onViewChange`
- `onSearchChange`
- `onSkillPreview`
- `onSkillToggle`
- `onProjectOpenChange`

`AwakenProjectSkillsDialog` 和 `AwakenSkillPreviewDialog` 同样不加载项目或文件；项目数据、`resources`、`resourcesTruncated`、`frontmatterRaw`、`markdown`、`loading` 和 `errorMessage` 均由 consumer 提供。

## 交互与可访问性

- 四个来源遵循 `Awaken Skills`、`项目 Skills`、`全局 Skills`、`Agents Skills` 的顺序。
- Agents Skills 将 consumer 提供的来源组扁平为一个直接可见的 Skill 卡片网格；组标签和路径只参与搜索，不渲染折叠标题、数量徽标或外层分组卡片。
- Skill 卡片主体与开关是两个独立可聚焦控件，切换开关不会打开预览。
- 开关使用 `role="switch"` 与 `aria-checked`；busy 和 disabled 状态不会触发回调。
- 项目文件夹的展开状态与项目 Dialog 保持一致。
- 预览包内容是 Dialog 内唯一纵向滚动容器；文件树、frontmatter 与正文共享同一内容流，Markdown 禁止 raw HTML、Mermaid 与外部图片请求。
- 浅色与暗色全部继承 `--aw-*` tokens，并尊重 `prefers-reduced-motion`。

## 边界

- 不依赖 Electron、runtime、HTTP、SSE、本地文件系统、业务 store 或 `window.awaken`。
- Gallery 只使用 mock 数据验证 UI 能力，不代表 Desktop 已经接入本组件族。
- Skill 的来源发现、启停策略、预览权限、文件读取、删除与错误恢复仍由 Desktop/Core owner 决定。
