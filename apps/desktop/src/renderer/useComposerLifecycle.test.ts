import { describe, expect, it } from "vitest";
import { composerRuntimePanelsForStatus } from "./useComposerLifecycle";

describe("composer credential recovery presentation", () => {
  it("labels credential failures as a model-key issue instead of a generic run error", () => {
    expect(
      composerRuntimePanelsForStatus({
        desktopNeedsRestart: false,
        desktopStarting: false,
        runtimeErrorMessage:
          "模型配置「Kimi Coding Plan」的 API key 无法从 ~/.awaken/.env 读取。",
        runtimeOperationIssue: {
          code: "credential_unreadable",
          message: "模型密钥无法读取",
          retryable: true,
          actions: ["retry", "reenter"],
        },
      }),
    ).toEqual([
      {
        id: "run-error",
        title: "模型密钥无法读取",
        message:
          "模型配置「Kimi Coding Plan」的 API key 无法从 ~/.awaken/.env 读取。",
        tone: "error",
        actions: ["retry", "reenter"],
      },
    ]);
  });
});
