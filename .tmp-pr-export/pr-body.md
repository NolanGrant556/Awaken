## 背景

针对当前 4 家厂商（DeepSeek、Kimi CN、Kimi Coding Plan、火山 Coding Plan）的五个缓存/成本可观测性与准确性问题：

1. 用户在 UI 里几乎看不见缓存效果——只能看到本地估算的上下文占用，看不到 hit/miss/cost；
2. 压缩阈值在猜——基于本地估算，不是 provider 真实用量，可能压太早（浪费）或压太晚（provider 报错）；
3. Prefix 变化默默打冷缓存，没有诊断——MCP/Skill/日期变化时缓存冷了，但不知道是哪段变了；
4. 火山 Coding Plan 的缓存合同没说清楚——不应和 DeepSeek/Kimi 一样标「确定支持」；
5. 成本只能算一部分——只有 DeepSeek 有可量化成本，Kimi/火山/custom 算不出具体成本。

## 改动

### 1. UI 显示 hit/miss/cost（任务 1+4+5）

`AwakenContextUsagePanel` 新增「上次请求」节，仅在 `ModelUsageRecord` 携带 cache/cost/invalidation 信号时渲染，避免 status-only 记录产生噪声：

- **缓存徽标**按 `cache_mode` + `cached_input_tokens` 联合判定：
  - `undocumented`（火山 Coding Plan）固定显示「未确认」，不声称命中或未命中——这是 provider 未确认缓存合同，区别于「provider 确认但本次缺字段」的「未知」
  - `automatic_prefix`/`affinity` 在 `cached_input_tokens` 缺失时显示「未知」，`> 0` 显示「已命中」，`= 0` 显示「未命中」
  - 无 `cache_mode` 不显示徽标
- **命中/未命中/输出 Token 行**仅在对应字段非 null 时显示；缺失不折 0、不显示
- **成本行**：`calculated_cost_usd` 非 null 显示 `$<amount>`（4 位小数）；为 null 显示「未知」，覆盖 Kimi Coding Plan、火山套餐与 custom endpoint 等无 token-to-plan 换算合同的情形
- **失效原因行**：`invalidation_reason` 非空时显示，承载 prefix 冷边界或 append-only host delta 的 segment 级诊断

### 2. 用「上次真实用量+本地新增」校正压缩判断（任务 2）

新增 `ContextWindowState.ShouldAutoCompactByActualUsage`：

- 当 `LastAPIUsageTokens > 0` 且 `LastAPIUsageTokens + EstimatedDeltaTokens >= AutoCompactThreshold` 时触发 proactive compact
- 与 v4 `EstimatedTokens` 触发是 **OR 关系**，不替换 `EstimatedTokens`，也不修改 `awaken_leaf_v4` 估算口径——`ContextActualTokens`、provider total/output 仍不参与当前窗口估算
- 补 v4 estimator 在 prefix 命中、tool result 体积差和媒体能力差异上的偏差，避免 provider 真实 input 已超窗口但本地估算仍低于阈值的过晚压缩
- 溢出时直接触发压缩作为防御
- 成功压缩后 governor 把 `lastAPIUsageTokens` 重置为 0，因为下一次请求是基于 compact 后的新 prefix，上一条 provider-reported usage 不再对应其 input，避免 stale-trigger 循环

### 3. Prefix segment digest 诊断写入 InvalidationReason（任务 3）

`updateContextEpoch` 增加 `changedSegments` 收集器：

- source 更新/删除/unavailable 与新动态工具追加都记下变化的 segment label
- append-only host delta 的 `BoundaryReason` 从 `append_only_host_delta` 升级为 `append_only_host_delta:<segment1>,<segment2>`
- segment label 为变化的 source key（`runtime_context`/`skills`/`memory`）或 `tool:<name>`（新接入的动态工具）
- 让「MCP/技能/日期变化导致缓存打冷」可以从 reason 直接定位到具体 segment，而不是一个笼统的 host delta
- `BoundaryReason` 写入 `ModelUsageRecord.InvalidationReason` 并在面板「失效原因」行展示

### 4. 火山 Coding Plan 合同明确标为未确认（任务 4）

- `cache_mode`「undocumented」映射为「未确认」徽标 tone，不与「未知」混淆
- 缓存治理文档 §5、§9 明确：火山是否存在透明缓存、是否返回稳定 cached usage，官方尚未确认；Awaken 不发送未文档化字段

### 5. Kimi/火山/custom 成本显示未知（任务 5）

- `calculated_cost_usd` 为 null 时面板显示「未知」而不折 0
- 缓存治理文档 §7 明确：Kimi Coding Plan、火山套餐和 custom endpoint 不做 token-to-plan 换算，`calculated_cost` 保持 unknown

## 文档

- `docs/02_主智能体闭环/提示词缓存与成本治理.md` §2.1、§6.2 同步面板「上次请求」节行为与 `BoundaryReason` segment 级诊断格式
- `docs/02_主智能体闭环/上下文窗口与压缩.md` §4 记录 `ShouldAutoCompactByActualUsage` 并行触发与 `lastAPIUsageTokens` 重置语义

## 测试

- `AwakenContextUsagePanel.states.ts` 新增 `cache-hit` / `cache-undocumented` 两个 demo states
- `App.test.ts` 新增 2 个测试：cache hit + cost 投影；undocumented provider 显示「未确认」+「未知」
- `context_engine_test.go` 新增 `TestShouldAutoCompactByActualUsage` 表驱动测试（6 cases）+ `TestPrepareRequestAutoCompactsByActualUsageEvenWhenV4EstimateIsBelowThreshold` + `TestPrepareRequestDoesNotCompactWhenBothEstimateAndActualAreBelowThreshold`
- `context_epoch_store_test.go` 新增 `TestContextEpochAppendOnlyDeltaReasonCarriesChangedSegments`

## 沙箱内自动化验证（本次通过）

- `go build ./...` ✓
- `go vet ./src/agentcore/... ./src/runtime/...` ✓
- `go test -count=1 ./src/agentcore/... ./src/runtime/...` ✓（agentcore 3.2s，runtime 29.7s）
- `npm --workspace @awaken/desktop run test` ✓（515/515，55 个文件）
- `npm run typecheck:desktop` ✓
- `npm run typecheck:ui` ✓
- `npm run smoke:ui` ✓（70 components）
- `git diff --check` ✓

## 不能在沙箱内完成的项（需 reviewer 本地验证）

1. **Desktop 真实验收 AC-CU-01~07**：Linux 沙箱无法跑 Electron Desktop。需在 macOS 上按 `docs/02_主智能体闭环/验收.md` 跑：开面板看「上次请求」节、缓存徽标、成本行、失效原因行；触发成功/失败 compact 看徽标切换；切 thread 重启看 v4 重建。
2. **真实 provider canary**：30 次 × 4 endpoint，需真实 API 额度。重点验证：DeepSeek 自动前缀缓存返回 `cached_tokens`、Kimi affinity key applied、Volcano 始终 unknown、custom endpoint 不发送私有字段。

## 不合并进 main

按用户要求，本 PR 仅提交到 `feat/cache-visibility` 分支，不合并进 main，等 reviewer 完成本地 Desktop 验收与 provider canary 后再决定是否合并。
