# Tasks

- [x] Task 1：projector 归并 read-only tools
  - [x] `read_file` 进入 exploring action
  - [x] `list_files` 进入 exploring action
  - [x] `grep_search` 进入 exploring action
  - [x] action id 由 toolCallId 稳定生成
  - [x] 工具失败时 action 显示 error
  - Task Dependencies：无

- [x] Task 2：Run Stage 渲染 exploring 摘要
  - [x] exploring item 渲染 action 列表
  - [x] 主叙事只显示动作 title/status/短 detail
  - [x] 主叙事不显示完整 tool result JSON 或大段输出
  - Task Dependencies：Task 1

- [x] Task 3：Inspector 保存 tool args/result
  - [x] `TOOL_CALL_ARGS` 累积 raw args
  - [x] `TOOL_CALL_RESULT` 保存 raw result
  - [x] 右侧 Latest Tool 显示 args 与 result
  - Task Dependencies：Task 1

- [x] Task 4：测试与验收
  - [x] reducer 测试覆盖 read/list/search 合并为一个 exploring item
  - [x] reducer 测试覆盖 replay 不重复 action
  - [x] reducer 测试覆盖 raw args/result 保存到 inspector record
  - [x] `npm --workspace @awaken/desktop run test`
  - [x] `npm run typecheck:ui`
  - [x] `npm run typecheck:desktop`
  - [x] `npm run build:desktop`
  - [x] `git diff --check`
  - [x] 真实 LLM read-only smoke 通过
  - [x] 独立 review pass，修复后复审
  - Task Dependencies：Task 1、Task 2、Task 3
