# Checklist

- [x] run 中 composer 主按钮显示 stop
- [x] run 中点击 stop 调用 cancel flow
- [x] cancel 后状态显示 cancelled，不显示 failed
- [x] done 后再次输入可 follow-up 到同一 thread
- [x] 空输入不能提交
- [x] Cmd/Ctrl+Enter 在可提交时仍触发 submit
- [x] composer 底部固定且不被 narrative 遮挡
- [x] `packages/ui` 未引入 runtime/Electron/AG-UI/A2UI/SSE/window.awaken
- [x] `npm --workspace @awaken/desktop run test` 通过
- [x] `npm run typecheck:ui` 通过
- [x] `npm run typecheck:desktop` 通过
- [x] `npm run build:desktop` 通过
- [x] `git diff --check` 通过
- [x] UI 视觉检查通过
- [x] 独立 review pass 未发现 blocker / correctness / acceptance finding
