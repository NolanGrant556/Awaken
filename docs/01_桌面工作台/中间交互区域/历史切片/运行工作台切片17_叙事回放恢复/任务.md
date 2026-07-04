# Tasks

- [x] 明确 run recovery action 的 prompt/thread label 输入
- [x] 调整 reducer 的 stream interruption/reconnecting 状态语义
- [x] 补齐 narrative replay idempotency 覆盖多 item 类型
- [x] 补齐 pending approval replay 与 terminal answer replay
- [x] 确认 runtime client reconnect 使用 last `after_seq` 并补测试
- [x] 增加 Desktop recovery smoke / reducer-client 验收
- [x] 跑 Desktop tests/typecheck/build、必要 Go tests、真实 recovery smoke、diff check
- [x] 独立 review 并修复 findings

## Task Dependencies

- spec/checklist 创建后再改代码。
- reducer action/payload 先于 App recovery 调用。
- replay tests 先于 projector/reducer 修复。
- reconnect tests 先于 client 行为调整。
- 全部测试和 recovery smoke 完成后再勾 checklist。
