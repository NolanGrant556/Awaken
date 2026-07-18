# Awaken 公开设计库调研

> 调研日期：2026-07-18  
> 性质：选型参考，非正式产品契约；不替代 `Awaken Design System` 与 `DESIGN_SYSTEM_GUIDE.md`。

---

## 1. 命题

Awaken 当前以自有 `@awaken/design-system` 为唯一运行时 UI 面，视觉参考源是仓库内镜像的 TraeWork（`Awaken Design System/reference/traework/`）。后续待办要求「优化前端整体设计，可参考苹果的设计与交互风格」。

因此需要回答：

**在不替换 Awaken 自有 Design System、不破坏 Desktop / Core / AG-UI ownership 的前提下，有没有比 TraeWork 更适合 Awaken 的公开设计库，可作为视觉、交互或 Workbench 组件形态的参考与取材源？**

筛选约束（来自当前仓库事实）：

| 约束 | 依据 |
| --- | --- |
| 浅色优先、中性表面、克制边框与字重层级 | `DESIGN_SYSTEM_GUIDE.md` |
| 主操作黑/近黑；不引入 TraeWork 紫 brand | 同上 |
| 避免装饰背景、glow、嵌套卡片、临时色板 | 同上 |
| DS 只接收 props/callbacks，不拥有业务状态与协议 | 根 `README.md` |
| 主界面是 Run Workbench：侧栏 + 叙事/工具迹线 + Composer + 交付物 | 根 `README.md`、桌面工作台文档 |
| macOS-first Electron Desktop | 根 `README.md` |
| 待办方向：苹果设计与交互 | `Awaken后续开发待办.md` |

---

## 2. 调研结果

### 2.1 候选库总览

| 库 | 类型 | 地址 | 与 Awaken 的匹配摘要 |
| --- | --- | --- | --- |
| Vercel AI Elements | AI 界面组件（shadcn registry） | https://elements.ai-sdk.dev/ | Message / Tool / Reasoning / PromptInput 与现有 Composer、ThoughtTrace、ToolTrace 高度同构 |
| prompt-kit | AI 界面组件（shadcn） | https://www.prompt-kit.com/ | 更轻；Steps、Chain of Thought、Tool 可对照 RunStage |
| assistant-ui | Chat 原语 + 可选 runtime | https://www.assistant-ui.com/ | 流式、滚动、Composer、工具展示成熟；带 runtime 假设 |
| LiqUIdify | Apple HIG React 组件库 | https://www.useliquidify.dev/ | 明确 iOS/macOS HIG、弹簧动效与材质；对齐「参考苹果」 |
| awesome-design-md | `DESIGN.md` 品牌视觉集合 | https://github.com/VoltAgent/awesome-design-md | Apple / Notion 等可作 Agent 视觉约束；Linear/Raycast 偏紫或深色 |
| StyleSeed | Agent 设计规则 + 组件/皮肤 | https://github.com/bitjaru/styleseed | 教 Agent「设计判断」；皮肤可换，但默认生态偏 shadcn |
| Atlas | Agent UI 设计语言（CSS） | https://ui.pacifio.dev/ | 密信息 agent 界面；默认 AMOLED/深色，与浅色优先冲突 |
| dock-ui | Agent-native 原语 | https://github.com/try-dock-ai/dock-ui | AgentOrb、Presence、Comment 等；产品隐喻偏协作 agent，非 Workbench |
| Houston | Agent 桌面应用框架（Tauri） | https://github.com/ja-818/houston | 整船 session/workspace，架构接管风险高 |
| CopilotKit | Agentic 前端框架 + AG-UI | https://www.copilotkit.ai/ | 能力完整，但会抢 AG-UI / generative UI ownership |

### 2.2 按适配维度的结论

#### A. 视觉与交互语言（对齐「苹果向」）

- **更合适**：LiqUIdify；awesome-design-md 中的 Apple / Notion 向 `DESIGN.md`。
- **需谨慎**：Linear（紫强调）、Raycast（深色 chrome）、Atlas（AMOLED 深色）——与现有「禁紫、浅色优先」冲突。
- **不适合整库替换**：任何完整第三方组件包都会与现有 `--aw-*` token、`Awaken*` 组件 API 冲突。

#### B. Workbench / Agent 叙事面组件形态

- **更合适**：AI Elements、prompt-kit——覆盖消息、工具调用、推理、输入框、步骤等 Awaken 已有或相邻的产品面。
- **可学不可接**：assistant-ui 的滚动锚定、流式体验、Composer 细节值得参考；其 runtime / thread 模型不应接入（Awaken 已自有 thread/run）。
- **不对齐**：dock-ui、Houston 的产品隐喻或桌面框架边界与 Awaken Desktop 现状不符。

#### C. 架构与 ownership 风险

| 方案 | 风险 |
| --- | --- |
| 整包接入 CopilotKit / Houston / assistant-ui runtime | 高：侵占 Desktop 状态、协议或会话生命周期 |
| 整包换成 LiqUIdify / shadcn 体系 | 高：重写 DS，打断现有 Gallery、验收与 Desktop 投影 |
| 继续加深 TraeWork 为唯一视觉源 | 中：已声明勿用紫 brand；再加深会持续拉偏 |
| 仅作 reference：镜像关键组件结构与交互，映射到 `--aw-*` | 低：符合当前 DS 边界 |

### 2.3 相对 TraeWork 的判断

| 维度 | TraeWork（现状） | 更优公开源 |
| --- | --- | --- |
| 与 Awaken 历史渊源 | 高（已镜像） | 无 |
| 浅色 / 中性克制 | 部分可用，但带紫 brand | prompt-kit / AI Elements（可主题化）；Apple/Notion DESIGN.md |
| 苹果向交互与材质 | 弱 | LiqUIdify |
| Agent Workbench 专用组件 | 弱（偏通用 UI kit） | AI Elements、prompt-kit |
| 作为运行时依赖 | 已明确禁止 | 同样应禁止；只作参考 |

**结论**：TraeWork 仍可保留为历史视觉样本，但不宜继续作为「优化前端整体设计」的主参考源。公开库里没有一份「开箱即 Awaken」的完整替代品；需要按层拆开取材。

---

## 3. 推荐

### 3.1 推荐组合（主建议）

继续以 `@awaken/design-system` 为唯一运行时组件面，公开库只做 **参考镜像 + 局部交互/结构迁移**，组合如下：

1. **视觉与交互语言**  
   - 主参考：[LiqUIdify](https://www.useliquidify.dev/)  
   - 辅助约束文件：[awesome-design-md](https://github.com/VoltAgent/awesome-design-md) 中的 Apple / Notion 向 `DESIGN.md`（避免 Linear 紫、Raycast 深色作为默认）

2. **Workbench 组件形态**  
   - 主参考：[Vercel AI Elements](https://elements.ai-sdk.dev/)  
   - 备选/轻量对照：[prompt-kit](https://www.prompt-kit.com/)  
   - 重点对照现有组件：`AwakenComposer`、`AwakenThoughtTrace`、`AwakenToolTrace*`、`AwakenNarrativeFeed` / RunStage 相关

3. **流式与输入体验细节**  
   - 仅 UX 参考：[assistant-ui](https://www.assistant-ui.com/)  
   - 不接入其 runtime、thread store 或 AI SDK 绑定层

### 3.2 明确不推荐作为主路径

- Atlas、Houston、dock-ui：视觉或架构与 Awaken 不对齐  
- CopilotKit：ownership 冲突成本过高  
- 用任一公开库整体替换 `@awaken/design-system`  
- 把 TraeWork 紫 brand 或深色 agent dashboard 审美直接升格为产品默认

### 3.3 建议的落地顺序（调研后续，非本次实施）

1. 建立对照表：AI Elements / prompt-kit 关键组件 ↔ 现有 `Awaken*` 组件（结构、状态、交互差异）。  
2. 选定 1 份 Apple/Notion 向 `DESIGN.md` 或 LiqUIdify token 摘要，写成「Awaken 视觉刷新约束」，与 `DESIGN_SYSTEM_GUIDE.md` 对齐后合并，避免双权威。  
3. 先改 token 与少数高频组件（Composer、ToolTrace、Message），Gallery + Desktop 真实验收后再扩大范围。

---

## 4. 一句话结论

**没有比 TraeWork「整库更合适」的公开 Design System 可直接换上；最适合 Awaken 的做法是：苹果向交互用 LiqUIdify / Apple·Notion DESIGN.md，Workbench 形态用 AI Elements（或 prompt-kit），流式细节参考 assistant-ui，全部映射进现有 `@awaken/design-system`，而不是换栈。**
