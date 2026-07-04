# Slice 7：文件变更 Artifact Spec

## Why

Slice 5/6 已完成 apply_patch 的审批与拒绝路径，但用户 Approve 后无法直观看到"改了哪些文件、改了什么"。本切片让 apply_patch 产出结构化 changed-file artifact（Added/Modified/Deleted/Moved），并让 Desktop 的 artifact panel 按变更类型展示，使文件修改变得可审计、可追溯。

## What Changes

- Tools：`apply_patch` 实现 `tool.ArtifactProducingTool` 接口，按 patch operation 产出 Added/Modified/Deleted/Moved artifact descriptor。
- UI 类型：扩展 `ArtifactItem`，新增 `changeType`、`path`、`kind`、`status` 字段，支持按变更类型区分展示。
- UI 组件：`ArtifactCard` 按 changeType 切换图标与色调（Added 绿、Modified 蓝、Deleted 红、Moved 紫）。
- Desktop：`App.tsx` 将 reducer 的 `ArtifactRecord` 直接映射到扩展后的 `ArtifactItem`，不再把 kind/status/path 塞进 description 字符串。
- 测试：apply_patch artifact 产出单测（Add/Update/Delete/Move）+ reject 不产 artifact 断言。

## Impact

- 受影响能力：文件变更 artifact 输出、artifact panel 展示。
- 受影响代码：
  - `src/tools/apply_patch/apply_patch.go`（新增 `Artifacts` 方法）
  - `src/tools/apply_patch/executor.go`（暴露 patch 分类信息）
  - `src/tools/apply_patch/types.go`（patch kind 与 movePath 已有）
  - `src/tool/tool.go`（`ArtifactDescriptor` 已有，无需改）
  - `packages/ui/src/types.ts`（扩展 `ArtifactItem`）
  - `packages/ui/src/components/feedback/ArtifactCard.tsx`（按 changeType 展示）
  - `apps/desktop/src/renderer/App.tsx`（映射 ArtifactRecord → ArtifactItem）
  - `apps/desktop/src/renderer/runtime/reducer.ts`（无需改，已支持）

## ADDED Requirements

### Requirement: apply_patch 产出 changed-file artifact

The system SHALL emit changed-file artifacts after apply_patch executes successfully.

#### Scenario: Create file produces Added artifact

- **WHEN** apply_patch creates a new file
- **THEN** an `AWAKEN_ARTIFACT_CREATED` event is emitted
- **AND** the artifact `kind` is `"added"`
- **AND** the artifact `path` is relative to the workspace root
- **AND** the artifact `title` contains the file path

#### Scenario: Modify file produces Modified artifact

- **WHEN** apply_patch modifies an existing file
- **THEN** an `AWAKEN_ARTIFACT_UPDATED` event is emitted
- **AND** the artifact `kind` is `"modified"`
- **AND** the artifact `path` is relative to the workspace root

#### Scenario: Delete file produces Deleted artifact

- **WHEN** apply_patch deletes a file
- **THEN** an `AWAKEN_ARTIFACT_UPDATED` event is emitted
- **AND** the artifact `kind` is `"deleted"`
- **AND** the artifact `path` is relative to the workspace root

#### Scenario: Move file produces Moved artifact

- **WHEN** apply_patch moves a file (update with non-empty movePath)
- **THEN** an `AWAKEN_ARTIFACT_UPDATED` event is emitted
- **AND** the artifact `kind` is `"moved"`
- **AND** the artifact `path` reflects the destination path relative to workspace root

### Requirement: Artifact path 与 workspace 一致

The system SHALL emit artifact paths that are relative to the workspace root, consistent with approval affected paths.

#### Scenario: Path consistency

- **WHEN** apply_patch produces artifacts
- **THEN** each artifact path matches the corresponding entry in the approval's `affectedPaths`
- **AND** paths use forward slashes regardless of OS

### Requirement: Reject 后不产生文件变更 artifact

The system SHALL NOT emit changed-file artifacts when an approval is rejected.

#### Scenario: Reject suppresses artifacts

- **WHEN** the user rejects an apply_patch approval
- **THEN** no `AWAKEN_ARTIFACT_CREATED` or `AWAKEN_ARTIFACT_UPDATED` event is emitted for the changed files
- **AND** the artifact panel does not show any new file change entries

### Requirement: UI 按变更类型展示 Artifact

The system SHALL render artifacts with distinct visual cues based on change type.

#### Scenario: ArtifactCard shows change type

- **WHEN** the artifact panel displays a changed-file artifact
- **THEN** Added artifacts show a green icon
- **AND** Modified artifacts show a blue icon
- **AND** Deleted artifacts show a red icon
- **AND** Moved artifacts show a purple icon
- **AND** the file path is displayed
- **AND** a short description or summary is shown

### Requirement: Artifact 是只读记录

The system SHALL treat artifacts as read-only records, not executable actions.

#### Scenario: Artifact is not executable

- **WHEN** the user views an artifact in the panel
- **THEN** the artifact card does not provide any execute/apply action
- **AND** clicking only shows a preview or summary

## MODIFIED Requirements

无

## REMOVED Requirements

无
