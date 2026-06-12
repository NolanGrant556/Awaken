# Awaken Agent Core 与 System Prompt 重构 - 产品需求文档

## Overview
- **Summary**: 基于设计文档重构 Awaken 项目架构，将 CLI、Agent Core、Prompt、Context、LLM 解耦，实现 System Prompt 与 Context 的分离管理，支持动态运行时信息注入。
- **Purpose**: 解决当前架构中 CLI、Agent、Context 职责不清的问题，建立可复用的 Agent Core，支持多入口（CLI、桌面端、后台任务）复用。
- **Target Users**: Awaken 开发者、CLI 用户、未来扩展入口的集成者

## Goals
- 将 System Prompt 从代码中分离到 Markdown 模板文件
- 实现 Context 只保存会话历史，不负责 system prompt
- 建立 Agent Core 作为唯一的请求组装层
- 支持动态运行时信息（cwd、date、model、tools）注入
- 优先支持 native tool calls，保留 XML action parser 作为兼容层

## Non-Goals (Out of Scope)
- 不复制 Claude Code 的完整目录结构
- 不一次性引入当前不需要的抽象
- 不改写 CLI 的交互逻辑（只调整内部调用方式）

## Background & Context
当前项目存在以下问题：
1. System Prompt 静态文本写在 Go 代码中
2. Context 会把 system prompt 作为第一条消息保存
3. 请求组装逻辑分散在多个模块
4. XML action parser 成为核心依赖

## Functional Requirements
- **FR-1**: 静态 System Prompt 存储在 Markdown 文件中
- **FR-2**: Context 只保存会话消息（user/assistant/tool），不包含 system prompt
- **FR-3**: Agent Core 负责组装 LLM 请求（system prompt + messages + tools）
- **FR-4**: 支持动态运行时信息注入（cwd、date、model、tools summary）
- **FR-5**: 优先处理 native tool calls，XML 作为 fallback

## Non-Functional Requirements
- **NFR-1**: 重构后所有测试通过（go test ./...）
- **NFR-2**: CLI 交互模式行为不变
- **NFR-3**: 模块间解耦，单一职责

## Constraints
- **Technical**: Go 语言，保持向后兼容
- **Dependencies**: 现有 LLM API、Context、Tool 模块

## Assumptions
- 现有代码结构基本合理，只需调整职责边界
- 开发者熟悉 Go 语言和项目结构

## Acceptance Criteria

### AC-1: 静态 Prompt 在 Markdown 文件中
- **Given**: 项目目录结构完整
- **When**: 查看 `src/prompt/templates/default_system.md`
- **Then**: 文件存在且包含静态 prompt 内容
- **Verification**: `programmatic`
- **Notes**: 使用 `rg` 检查 CLI 目录不包含硬编码的 prompt 文本

### AC-2: Context 不存 System Prompt
- **Given**: 调用 `Context.GetMessages()`
- **When**: 获取消息列表
- **Then**: 返回的消息不包含 system role 的消息
- **Verification**: `programmatic`

### AC-3: Agent Core 统一组装请求
- **Given**: 执行 Agent.Run()
- **When**: 调用 LLM
- **Then**: 请求通过 `request_builder.go` 组装，包含 system prompt、messages、tools
- **Verification**: `programmatic`

### AC-4: 动态信息每轮构建
- **Given**: 修改 cwd/model/date
- **When**: 执行 Agent.Run()
- **Then**: effective system prompt 包含更新后的动态信息
- **Verification**: `programmatic`

### AC-5: Native Tool Calls 优先
- **Given**: LLM 返回包含 native tool_calls
- **When**: Agent 处理响应
- **Then**: 优先解析 native tool_calls，XML 作为 fallback
- **Verification**: `programmatic`

### AC-6: 旧功能不回退
- **Given**: 重构完成后
- **When**: 运行 `go test ./...`
- **Then**: 所有测试通过
- **Verification**: `programmatic`

## Open Questions
- [ ] 动态信息的更新频率（每轮调用 vs 缓存）
- [ ] XML action parser 的删除时机