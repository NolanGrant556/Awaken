import { describe, expect, it } from "vitest";
import { composerRuntimePanelsForStatus } from "./useComposerLifecycle";

describe("composer credential recovery presentation", () => {
  it("labels credential failures as a model-key issue instead of a generic run error", () => {
    expect(
      composerRuntimePanelsForStatus({
        desktopNeedsRestart: false,
        desktopStarting: false,
        runtimeErrorMessage:
          "模型配置「Kimi Coding Plan」的安全凭据服务暂时不可用，凭据仍已保留。请重新启动 Awaken 后重试。",
        runtimeOperationIssue: {
          code: "credential_temporarily_unavailable",
          message: "模型密钥暂时不可用",
          retryable: true,
          actions: ["relaunch"],
        },
      }),
    ).toEqual([
      {
        id: "run-error",
        title: "模型密钥暂不可用",
        message:
          "模型配置「Kimi Coding Plan」的安全凭据服务暂时不可用，凭据仍已保留。请重新启动 Awaken 后重试。",
        tone: "error",
        actions: ["relaunch"],
      },
    ]);
  });
});
