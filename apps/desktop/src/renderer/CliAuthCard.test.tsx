import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CliAuthCard } from "./CliAuthCard";

describe("CliAuthCard", () => {
  it("renders a fixed QR resource without narrative data", () => {
    const html = renderToString(
      <CliAuthCard
        session={{
          id: "session-1",
          provider: "lark-cli",
          phase: "waiting_user",
          qrImageUrl: "awaken-cli-auth://lark-cli/session-1/qr.png",
          safeMessage: "请扫码",
          recoveries: ["refresh_qr", "cancel"],
        }}
        onCancel={vi.fn()}
        onRetry={vi.fn()}
      />,
    );
    expect(html).toContain("width=\"224\"");
    expect(html).toContain("height=\"224\"");
    expect(html).toContain("awaken-cli-auth://lark-cli/session-1/qr.png");
    expect(html).toContain("重新获取");
  });
});
