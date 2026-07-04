# Tasks

- [x] 新增内部 `ToolCallArgsDelta` / `ToolProgress` 事件
- [x] Provider stream 发出 args delta，同时保留完整 args fallback
- [x] Tool runtime 发出 progress
- [x] AG-UI mapper 映射 args delta/progress
- [x] Desktop shared/reducer/projector 支持 args delta/progress
- [x] 增加 provider/runner/mapper/reducer 测试
- [x] 运行 Go tests、Desktop tests/typecheck/build、真实 LLM tool smoke、diff check
- [x] 独立 review 并修复 findings

## Task Dependencies

- 内部事件类型先于 mapper/Desktop 改动。
- Provider delta 与 runtime progress 可并行实现。
- Desktop projector 更新后再补 reducer 测试。
- 全部测试和真实 smoke 完成后再勾 checklist。
