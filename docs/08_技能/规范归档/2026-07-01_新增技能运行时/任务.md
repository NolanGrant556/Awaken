## 1. 包结构与基础类型

- [ ] 1.1 创建 `src/skills/` 包：`types.go`、`registry.go`、`loader.go`、`frontmatter.go`、`bundled.go`
- [ ] 1.2 创建 `src/skillruntime/` 包：`tool.go`、`executor.go`、`fork.go`、`args.go`
- [ ] 1.3 定义 `SkillDefinition` struct：name、displayName、description、whenToUse、allowedTools、arguments、model、effort、context（inline/fork）、agent、hooks、requiredConnectors、sourceOrigin
- [ ] 1.4 定义 `SkillState`：`available | unavailable | shadowed`，以及 `unavailableReason`

## 2. Frontmatter 解析与加载

- [ ] 2.1 实现 YAML frontmatter 解析（接受 `---` 包裹 + Markdown 正文）
- [ ] 2.2 schema 校验：必填 name/description，可选字段类型校验
- [ ] 2.3 `LoadDir(path, source)`：遍历目录加载所有 `.md`，单文件失败仅跳过该条
- [ ] 2.4 来源优先级合并：`project > user > bundled`，被遮蔽 entry 保留 `shadowedBy`
- [ ] 2.5 单测：合法 / 缺字段 / 非法 YAML / 同名覆盖 四种 case

## 3. 内置 Skill（骨架）

- [ ] 3.1 `src/skills/bundled/meeting-notes-distribute.md`：合法 frontmatter + TODO 文案
- [ ] 3.2 `src/skills/bundled/customer-followup-daily.md`
- [ ] 3.3 `src/skills/bundled/sheet-anomaly-scan.md`
- [ ] 3.4 `src/skills/bundled/doc-to-actions.md`
- [ ] 3.5 用 `go:embed` 把 bundled 目录编译进二进制

## 4. SkillTool 注册到工具池

- [ ] 4.1 实现 `SkillTool`（每个 available skill 一个独立工具，命名 `skill__{id}`）
- [ ] 4.2 tool description 自动用 `whenToUse`，input schema 用 `arguments` 定义
- [ ] 4.3 在 `src/tool/registry.go` 启动注册，并订阅 SkillRegistry 变更（`unavailable` → 反注册）
- [ ] 4.4 单测：connector 缺失时对应 skill 不出现在 tool list

## 5. 参数替换

- [ ] 5.1 实现 `${arg}` 占位符替换；保留转义 `\${...}`
- [ ] 5.2 缺必填参数返回结构化错误，不调用 LLM
- [ ] 5.3 单测：基础替换 / 默认值 / 类型校验 / 转义

## 6. 执行器

- [ ] 6.1 实现 `Inline` 执行：把渲染后的正文作为 user message append 到主 run；返回主 run 的下一步事件流
- [ ] 6.2 实现 `Fork` 执行：调用 `runtime.RunService.StartChild`（接口缺失时退化为 inline 并记录 warning）
- [ ] 6.3 子 run 完成后发 `AWAKEN_SKILL_COMPLETED` 事件
- [ ] 6.4 单测：inline 执行通路；fork 退化通路

## 7. requiredConnectors 与可用性

- [ ] 7.1 SkillRegistry 订阅 connector + mcp-runtime 状态变更
- [ ] 7.2 一个 requiredConnector 缺失 → 该 skill 转 `unavailable`，原因带 connector 名
- [ ] 7.3 connector 恢复 → 自动转 `available` 并重新注册到工具池
- [ ] 7.4 单测：连/断 connector 时 skill 可用性切换

## 8. Server HTTP API

- [ ] 8.1 `GET /v1/skills`：返回 id、displayName、state、unavailableReason、sourceOrigin、shadowedBy
- [ ] 8.2 `POST /v1/skills/{id}/execute`：直接触发（参数走 body），返回 run id；用于测试和未来 UI
- [ ] 8.3 路由注册到 `src/server/router.go`

## 9. 与 agentcore 集成

- [ ] 9.1 agentcore 调用 SkillTool 时透传 `skill_id` 到事件流（用于审计 / observability）
- [ ] 9.2 在 system prompt 拼装阶段提示 LLM："Skill 是经过验证的方法，优先使用"
- [ ] 9.3 真 LLM smoke：扩展 `src/server` smoke 用例，让 agent 通过 SkillTool 跑一个 bundled skill 的最小通路

## 10. 文档

- [ ] 10.1 `docs/skill-md-format.md`：SKILL.md 完整 schema 说明 + 示例
- [ ] 10.2 README 增加"写一个自己的 Skill" 段
- [ ] 10.3 更新 `CLAUDE.md`，在工具层段落加入 Skill
