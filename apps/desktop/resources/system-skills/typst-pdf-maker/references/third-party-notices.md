# 第三方说明

本 Skill 的中文说明、Python 脚本和 Typst 主题资产为 Awaken 专用 clean-room 实现，不包含原参考包的代码或品牌内容。

## Typst CLI

- 项目：https://github.com/typst/typst
- 固定安装版本：0.15.0
- 许可证：Apache License 2.0
- 本 Skill 不分发 Typst 二进制；只有在用户任务执行时缺失 CLI，Agent 才从 Typst 官方 GitHub Release 下载相应平台资产并校验固定 SHA-256。

## Typst Universe

`routing-catalog.json` 仅保存公开包名、固定版本和路由元数据。第三方模板和包不随 Skill 分发，由 Typst 在用户明确选择后从 Typst Universe 获取。各包适用其自身许可证；对外分发生成物前，应按实际采用的包核对许可证和署名要求。

Markdown 适配入口会按需获取：

- `@preview/cmarker:0.1.10`
- `@preview/mitex:0.2.7`

本说明不把“可以下载”解释为“已完成全部第三方许可证审查”。
