import { describe, expect, it, vi } from "vitest";

vi.mock("electron", () => ({ safeStorage: {} }));

import {
  desktopOperationIssue,
  runDesktopOperation,
} from "./desktop-operation.js";
import { SecureCredentialError } from "./secure-credential-store.js";

describe("desktop operation result boundary", () => {
  it("turns temporary credential failures into an in-app relaunch action", () => {
    expect(
      desktopOperationIssue(
        new SecureCredentialError(
          "temporarily_unavailable",
          "安全凭据服务暂时不可用，请重新启动 Awaken 后重试。",
          true,
        ),
      ),
    ).toEqual({
      code: "credential_temporarily_unavailable",
      message: "安全凭据服务暂时不可用，请重新启动 Awaken 后重试。",
      retryable: true,
      actions: ["relaunch"],
    });
  });

  it("routes unreadable credentials to in-app re-entry", () => {
    expect(
      desktopOperationIssue(
        new SecureCredentialError("decrypt_failed", "请重新保存 API key。"),
      ),
    ).toMatchObject({
      code: "credential_unreadable",
      retryable: false,
      actions: ["reenter"],
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
        throw new SecureCredentialError("decrypt_failed", "请重新保存 API key。");
      }),
    ).resolves.toMatchObject({
      ok: false,
      issue: { code: "credential_unreadable" },
    });
  });
});
