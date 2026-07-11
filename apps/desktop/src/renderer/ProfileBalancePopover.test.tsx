import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  ProfileBalancePopover,
  credentialRecoveryForBalances,
} from "./ProfileBalancePopover";
import type { RuntimeClient } from "./runtime/client";
import type { ModelProfileBalance } from "../shared/runtime";

function createMockClient(): RuntimeClient {
  return {
    source: "electron",
    platform: "windows",
    getModelProfileBalances: vi.fn(async () => ({ balances: [] })),
  } as unknown as RuntimeClient;
}

const mockBalance: ModelProfileBalance = {
  profileId: "deepseek-1",
  providerId: "deepseek",
  modelId: "deepseek-v4-pro",
  displayName: "DeepSeek",
  status: "available",
  fetchedAt: "2026-01-01T00:00:00.000Z",
  detail: {
    kind: "amount",
    amounts: [
      { currency: "USD", total: 50, granted: 10, toppedUp: 40 },
    ],
  },
};

describe("ProfileBalancePopover", () => {
  it("renders nothing when closed", () => {
    const anchorRef = { current: null };
    const html = renderToString(
      <ProfileBalancePopover
        open={false}
        anchorRef={anchorRef}
        runtimeClient={createMockClient()}
        onOpenSettings={() => {}}
        onClose={() => {}}
      />,
    );
    expect(html).toBe("");
  });

  it("renders the empty state and in-app settings action when open", () => {
    const anchorRef = { current: null };
    const html = renderToString(
      <ProfileBalancePopover
        open
        anchorRef={anchorRef}
        runtimeClient={createMockClient()}
        onOpenSettings={() => {}}
        onClose={() => {}}
      />,
    );
    expect(html).toContain("没有已启用且有 API key 的模型");
    expect(html).toContain("设置");
  });

  it("collapses repeated provider failures into one credential recovery state", () => {
    expect(
      credentialRecoveryForBalances([
        {
          ...mockBalance,
          status: "error",
          credentialIssue: "unreadable",
        },
        {
          ...mockBalance,
          profileId: "kimi-1",
          providerId: "kimi-coding-plan",
          credentialIssue: "temporarily_unavailable",
        },
      ]),
    ).toBe("temporarily_unavailable");
  });
});
