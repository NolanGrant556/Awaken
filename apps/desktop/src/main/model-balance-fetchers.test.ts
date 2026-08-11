import { describe, expect, it, vi } from "vitest";
import {
  fetchDeepSeekBalance,
  fetchKimiBalance,
  fetchKimiCodingBalance,
  fetchOpenAIOrganizationUsage,
  fetchAnthropicOrganizationUsage,
  BalanceFetchError,
} from "./model-balance-fetchers.js";

describe("official organization usage", () => {
  it("uses an OpenAI admin key and aggregates paginated usage and cost", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer openai-admin");
      const isCost = url.includes("/organization/costs");
      const page2 = url.includes("page=next");
      return new Response(JSON.stringify({
        data: [{ results: [isCost
          ? { amount: { value: page2 ? 0.5 : 1.25, currency: "usd" } }
          : { input_tokens: page2 ? 3 : 10, input_cached_tokens: page2 ? 1 : 4, output_tokens: page2 ? 2 : 5, num_model_requests: 1 },
        ] }],
        next_page: page2 ? null : "next",
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    }));

    await expect(fetchOpenAIOrganizationUsage("openai-admin")).resolves.toMatchObject({
      kind: "usage",
      metrics: expect.arrayContaining([
        { name: "近 7 天输入", value: 13, unit: "tokens" },
        { name: "近 7 天成本", value: 1.75, unit: "usd" },
      ]),
    });
  });

  it("uses an Anthropic admin key and converts fractional cents to USD", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const headers = new Headers(init?.headers);
      expect(headers.get("x-api-key")).toBe("anthropic-admin");
      expect(headers.get("anthropic-version")).toBe("2023-06-01");
      const result = url.includes("cost_report")
        ? { amount: "123.45", currency: "USD" }
        : { uncached_input_tokens: 10, cache_read_input_tokens: 4, cache_creation: { ephemeral_5m_input_tokens: 2 }, output_tokens: 5 };
      return new Response(JSON.stringify({ data: [{ results: [result] }], has_more: false }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }));

    await expect(fetchAnthropicOrganizationUsage("anthropic-admin")).resolves.toMatchObject({
      kind: "usage",
      metrics: expect.arrayContaining([
        { name: "近 7 天缓存写入", value: 2, unit: "tokens" },
        { name: "近 7 天成本", value: 1.2345, unit: "usd" },
      ]),
    });
  });
});

describe("fetchDeepSeekBalance", () => {
  it("parses USD balance with granted and topped-up breakdown", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({
            is_available: true,
            balance_infos: [
              {
                currency: "USD",
                total_balance: "50.00",
                granted_balance: "10.00",
                topped_up_balance: "40.00",
              },
            ],
          }),
        } as Response),
      ),
    );

    const detail = await fetchDeepSeekBalance(
      "https://api.deepseek.com",
      "sk-test",
    );

    expect(detail.kind).toBe("amount");
    const amountDetail = detail as { kind: "amount"; amounts: Array<{ currency: string; total: number; granted?: number; topped_up?: number }> };
    expect(amountDetail.amounts).toEqual([
      { currency: "USD", total: 50, granted: 10, topped_up: 40 },
    ]);
  });

  it("prefers USD over CNY when multiple currencies are present", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({
            is_available: true,
            balance_infos: [
              {
                currency: "CNY",
                total_balance: "100.00",
                granted_balance: "0.00",
                topped_up_balance: "100.00",
              },
              {
                currency: "USD",
                total_balance: "30.00",
                granted_balance: "5.00",
                topped_up_balance: "25.00",
              },
            ],
          }),
        } as Response),
      ),
    );

    const detail = await fetchDeepSeekBalance(
      "https://api.deepseek.com",
      "sk-test",
    );

    const amountDetail = detail as { kind: "amount"; amounts: Array<{ currency: string }> };
    expect(amountDetail.amounts[0]?.currency).toBe("USD");
    expect(amountDetail.amounts[1]?.currency).toBe("CNY");
  });

  it("throws BalanceFetchError on 401", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Promise.resolve({
          ok: false,
          status: 401,
        } as Response),
      ),
    );

    await expect(
      fetchDeepSeekBalance("https://api.deepseek.com", "sk-test"),
    ).rejects.toBeInstanceOf(BalanceFetchError);
  });

  it("throws BalanceFetchError when balance_infos is empty", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({ is_available: true, balance_infos: [] }),
        } as Response),
      ),
    );

    await expect(
      fetchDeepSeekBalance("https://api.deepseek.com", "sk-test"),
    ).rejects.toBeInstanceOf(BalanceFetchError);
  });
});

describe("fetchKimiCodingBalance", () => {
  it("parses weekly quota and 5-hour window", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({
            usage: {
              limit: "2048",
              used: "214",
              remaining: "1834",
              resetTime: "2026-01-09T15:23:13.716839300Z",
            },
            limits: [
              {
                window: { duration: 300, timeUnit: "TIME_UNIT_MINUTE" },
                detail: {
                  limit: "200",
                  used: "139",
                  remaining: "61",
                  resetTime: "2026-01-06T13:33:02.717479433Z",
                },
              },
            ],
          }),
        } as Response),
      ),
    );

    const detail = await fetchKimiCodingBalance(
      "https://api.kimi.com/coding/v1",
      "kimi-test",
    );

    expect(detail.kind).toBe("quota");
    const quotaDetail = detail as { kind: "quota"; quotas: Array<{ name: string; limit: number; used: number; remaining: number; reset_at?: string }> };
    expect(quotaDetail.quotas).toEqual([
      {
        name: "本周请求",
        limit: 2048,
        used: 214,
        remaining: 1834,
        reset_at: "2026-01-09T15:23:13.716839300Z",
      },
      {
        name: "5 小时请求",
        limit: 200,
        used: 139,
        remaining: 61,
        reset_at: "2026-01-06T13:33:02.717479433Z",
      },
    ]);
  });

  it("derives used quota when Kimi omits the used field", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({
            usage: {
              limit: "2048",
              remaining: "1834",
              resetTime: "2026-01-09T15:23:13.716839300Z",
            },
            limits: [
              {
                window: { duration: 300, timeUnit: "TIME_UNIT_MINUTE" },
                detail: {
                  limit: "200",
                  remaining: "61",
                  resetTime: "2026-01-06T13:33:02.717479433Z",
                },
              },
            ],
          }),
        } as Response),
      ),
    );

    const detail = await fetchKimiCodingBalance(
      "https://api.kimi.com/coding/v1",
      "kimi-test",
    );

    expect(detail.kind).toBe("quota");
    const quotaDetail = detail as { kind: "quota"; quotas: Array<{ name: string; limit: number; used: number; remaining: number; reset_at?: string }> };
    expect(quotaDetail.quotas).toEqual([
      {
        name: "本周请求",
        limit: 2048,
        used: 214,
        remaining: 1834,
        reset_at: "2026-01-09T15:23:13.716839300Z",
      },
      {
        name: "5 小时请求",
        limit: 200,
        used: 139,
        remaining: 61,
        reset_at: "2026-01-06T13:33:02.717479433Z",
      },
    ]);
  });

  it("ignores non-5-hour window limits", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({
            usage: {
              limit: "2048",
              used: "0",
              remaining: "2048",
            },
            limits: [
              {
                window: { duration: 60, timeUnit: "TIME_UNIT_MINUTE" },
                detail: { limit: "100", used: "0", remaining: "100" },
              },
            ],
          }),
        } as Response),
      ),
    );

    const detail = await fetchKimiCodingBalance(
      "https://api.kimi.com/coding/v1",
      "kimi-test",
    );

    const quotaDetail = detail as { kind: "quota"; quotas: Array<{ name: string }> };
    expect(quotaDetail.quotas).toHaveLength(1);
    expect(quotaDetail.quotas[0]?.name).toBe("本周请求");
  });

  it("throws BalanceFetchError on invalid response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({ unexpected: true }),
        } as Response),
      ),
    );

    await expect(
      fetchKimiCodingBalance("https://api.kimi.com/coding/v1", "kimi-test"),
    ).rejects.toBeInstanceOf(BalanceFetchError);
  });

  it("throws BalanceFetchError on 403", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Promise.resolve({
          ok: false,
          status: 403,
        } as Response),
      ),
    );

    await expect(
      fetchKimiCodingBalance("https://api.kimi.com/coding/v1", "kimi-test"),
    ).rejects.toBeInstanceOf(BalanceFetchError);
  });
});

describe("fetchKimiBalance", () => {
  it("parses the official available, voucher, and cash balance response", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      expect(String(input)).toBe("https://api.moonshot.cn/v1/users/me/balance");
      expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer kimi-key");
      return new Response(JSON.stringify({
        code: 0,
        data: {
          available_balance: 49.58894,
          voucher_balance: 46.58893,
          cash_balance: 3.00001,
        },
        scode: "0x0",
        status: true,
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      fetchKimiBalance("https://api.moonshot.cn/v1", "kimi-key"),
    ).resolves.toEqual({
      kind: "amount",
      amounts: [{
        currency: "CNY",
        total: 49.58894,
        granted: 46.58893,
        topped_up: 3.00001,
      }],
    });
  });
});
