import { describe, expect, it } from "vitest";

import {
  desktopOperationIssue,
  runDesktopOperation,
} from "./desktop-operation.js";
import { AwakenEnvStoreError } from "./awaken-env-store.js";
import { AgentPlanAccountStoreError } from "./agent-plan-account-store.js";

describe("desktop operation result boundary", () => {
  it("routes an unreadable .env to retry or API key re-entry", () => {
    expect(
      desktopOperationIssue(
        new AwakenEnvStoreError("unreadable", "无法读取 ~/.awaken/.env 中的 API key。"),
      ),
    ).toEqual({
      code: "credential_unreadable",
      message: "无法读取 ~/.awaken/.env 中的 API key。",
      retryable: true,
      actions: ["retry", "reenter"],
    });
  });

  it("maps .env write failures to a generic operation failure", () => {
    expect(
      desktopOperationIssue(
        new AwakenEnvStoreError("write_failed", "must not cross IPC"),
      ),
    ).toEqual({
      code: "operation_failed",
      message: "Awaken 操作未完成，请稍后重试。",
      retryable: true,
      actions: ["retry"],
    });
  });

  it("routes unreadable Agent Plan account metadata to the credential recovery flow", () => {
    expect(
      desktopOperationIssue(
        new AgentPlanAccountStoreError("unreadable", "Agent Plan 账户元数据不可读。"),
      ),
    ).toEqual({
      code: "credential_unreadable",
      message: "Agent Plan 账户元数据不可读。",
      retryable: true,
      actions: ["retry", "reenter"],
    });
  });

  it("does not expose unknown downstream error text to the renderer", () => {
    expect(
      desktopOperationIssue(
        new Error("provider failed with Authorization: Bearer secret-value"),
      ).message,
    ).toBe("Awaken 操作未完成，请稍后重试。");
  });

  it("maps stale Skill selection failures without exposing Core paths", () => {
    const error = Object.assign(new Error("selected skill source changed: /secret/path"), {
      name: "RuntimeDomainError",
      code: "INVALID_COMPOSER_TOKEN",
      status: 400,
    });
    expect(desktopOperationIssue(error)).toEqual({
      code: "composer_selection_invalid",
      message: "选择的 Skill 已不可用或已经发生变化，请重新选择后再运行。",
      retryable: false,
      actions: [],
    });
  });

  it("distinguishes unavailable Plugin components from generic input failures", () => {
    const error = Object.assign(new Error("input is required: composer Plugin Agent is unavailable"), {
      name: "RuntimeDomainError",
      code: "INVALID_INPUT",
      status: 400,
    });
    expect(desktopOperationIssue(error)).toMatchObject({
      code: "plugin_component_unavailable",
      retryable: false,
    });
  });

  it("preserves a typed stale file comment without exposing file content", () => {
    expect(
      desktopOperationIssue(Object.assign(new Error("internal path must not cross IPC"), {
        name: "RuntimeDomainError",
        code: "FILE_COMMENT_STALE",
        details: { comment_id: "comment-1", reason: "digest_mismatch" },
      })),
    ).toEqual({
      code: "file_comment_stale",
      message: "文件内容已变化，请重新选择行并添加评论。",
      retryable: true,
      actions: ["retry"],
      commentId: "comment-1",
      reason: "digest_mismatch",
    });
  });

  it("returns a serializable result instead of throwing across Electron IPC", async () => {
    await expect(
      runDesktopOperation(async () => {
        throw new AwakenEnvStoreError("unreadable", "请重新输入 API key。");
      }),
    ).resolves.toMatchObject({
      ok: false,
      issue: { code: "credential_unreadable" },
    });
  });
});
