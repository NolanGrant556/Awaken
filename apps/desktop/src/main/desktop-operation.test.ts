import { describe, expect, it } from "vitest";

import {
  desktopOperationIssue,
  runDesktopOperation,
} from "./desktop-operation.js";
import { AwakenEnvStoreError } from "./awaken-env-store.js";

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

  it("does not expose unknown downstream error text to the renderer", () => {
    expect(
      desktopOperationIssue(
        new Error("provider failed with Authorization: Bearer secret-value"),
      ).message,
    ).toBe("Awaken 操作未完成，请稍后重试。");
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
