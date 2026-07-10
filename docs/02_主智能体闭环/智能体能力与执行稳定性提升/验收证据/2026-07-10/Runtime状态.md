# Primary Runtime 状态

记录时间：2026-07-10 15:14（Asia/Shanghai）

命令：`node scripts/check-primary-runtime.mjs --json`

```text
ready: true
schemaVersion: 2
bundleVersion: 0.2.0
platform: darwin
arch: arm64
active: 0.2.0/darwin-arm64/build-mrektar3-1dce472ed7
previous: 0.2.0/darwin-arm64/build-mrek49st-829e0c3487
provenance.ok: true
provenance.issues: []
missing: []

python:       ok — python-pptx and Pillow available
node:         ok — v24.3.0
libreoffice:  ok — LibreOffice 26.2.2.2
poppler:      ok — pdftoppm and pdfinfo available
ffmpeg:       ok — ffmpeg available
helper-packages: ok
```

生产根：`~/Library/Application Support/@awaken/desktop/runtimes/primary-runtime`

结论：active revision 完全位于 Awaken-owned runtime root；不存在 Codex cache wrapper 或外部 target，所有必需组件和 provenance probe 通过。
