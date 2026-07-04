# Checklist

- [x] 左侧 run/thread item 支持用户可读状态
- [x] 新 run 创建后左侧显示 running
- [x] awaiting approval 映射为 needs input
- [x] artifact/file change 后左侧可显示 ready for review
- [x] terminal finished run 显示 done，不保留 running
- [x] thread 切换后 answer narrative 保留
- [x] thread 切换后 approval narrative 保留
- [x] thread 切换后 artifact/fileChange narrative 保留
- [x] 左侧不再承担 Dashboard 或卡片墙职责
- [x] `packages/ui` 未引入 runtime/Electron/AG-UI/A2UI/SSE/window.awaken
- [x] `npm --workspace @awaken/desktop run test` 通过
- [x] `npm run typecheck:ui` 通过
- [x] `npm run typecheck:desktop` 通过
- [x] `npm run build:desktop` 通过
- [x] `git diff --check` 通过
- [x] UI 视觉检查通过
- [x] 独立 review pass 未发现 blocker / correctness / acceptance finding
