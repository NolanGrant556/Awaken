import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { ExternalActionGate } from "./ExternalActionGate";

describe("ExternalActionGate", () => {
  it("renders a safe HTTPS link and fixed QR frame without narrative data", () => {
    const html = renderToString(
      <ExternalActionGate
        interaction={{
          id: "ix-1",
          title: "钉钉授权",
          safeMessage: "完成钉钉授权后继续",
          verificationUrl: "https://login.dingtalk.com/oauth2/auth",
          provider: "dingtalk-workspace",
          recoveries: ["open_url", "confirm_done", "cancel"],
        }}
        onConfirmDone={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(html).toContain("external-action-gate");
    expect(html).toContain("钉钉授权");
    expect(html).toContain("https://login.dingtalk.com/oauth2/auth");
    expect(html).toContain("cli-auth-qr-frame");
    expect(html).toContain("我已完成");
    expect(html).toContain("取消");
  });
});
