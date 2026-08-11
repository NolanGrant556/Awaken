import type {
  ModelBalanceAmountWire,
  ModelBalanceDetailWire,
  ModelBalanceQuotaWire,
} from "../shared/runtime.js";

function trimTrailingSlash(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

function parseBalanceValue(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string") {
    const parsed = parseFloat(value);
    return Number.isNaN(parsed) ? 0 : parsed;
  }
  return 0;
}

class BalanceFetchError extends Error {
  constructor(
    message: string,
    public readonly status?: number,
  ) {
    super(message);
    this.name = "BalanceFetchError";
  }
}

async function fetchWithAuth(
  url: string,
  apiKey: string,
  signal?: AbortSignal,
): Promise<Response> {
  const response = await fetch(url, {
    method: "GET",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${apiKey}`,
    },
    signal,
  });
  return response;
}

async function assertOk(response: Response): Promise<void> {
  if (response.ok) return;
  if (response.status === 401) {
    throw new BalanceFetchError("API key 无效或未授权", 401);
  }
  if (response.status === 403) {
    throw new BalanceFetchError("API key 无权访问余额接口", 403);
  }
  if (response.status === 429) {
    throw new BalanceFetchError("请求过于频繁，请稍后重试", 429);
  }
  throw new BalanceFetchError(
    `服务返回 HTTP ${response.status}`,
    response.status,
  );
}

function assertObject(value: unknown): Record<string, unknown> {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  throw new BalanceFetchError("响应格式无效：期望对象");
}

export async function fetchDeepSeekBalance(
  baseURL: string,
  apiKey: string,
  signal?: AbortSignal,
): Promise<ModelBalanceDetailWire> {
  const normalizedBaseURL = trimTrailingSlash(baseURL);
  const response = await fetchWithAuth(
    `${normalizedBaseURL}/user/balance`,
    apiKey,
    signal,
  );
  await assertOk(response);

  const body = assertObject(await response.json());
  const balanceInfos = Array.isArray(body.balance_infos)
    ? body.balance_infos
    : [];

  const amounts: ModelBalanceAmountWire[] = balanceInfos
    .map((entry): ModelBalanceAmountWire | null => {
      const item = assertObject(entry);
      const currency =
        typeof item.currency === "string" ? item.currency.toUpperCase() : "";
      if (!currency) return null;
      return {
        currency,
        total: parseBalanceValue(item.total_balance),
        granted: parseBalanceValue(item.granted_balance),
        topped_up: parseBalanceValue(item.topped_up_balance),
      };
    })
    .filter((item): item is ModelBalanceAmountWire => item !== null);

  if (amounts.length === 0) {
    throw new BalanceFetchError("余额响应为空");
  }

  const usdIndex = amounts.findIndex((a) => a.currency === "USD");
  const orderedAmounts =
    usdIndex >= 0
      ? [amounts[usdIndex], ...amounts.slice(0, usdIndex), ...amounts.slice(usdIndex + 1)]
      : amounts;

  return { kind: "amount", amounts: orderedAmounts };
}

type KimiUsageDetail = {
  limit: string | number;
  used?: string | number;
  remaining: string | number;
  resetTime?: string;
};

type KimiWindowLimit = {
  window: {
    duration: number;
    timeUnit: string;
  };
  detail: KimiUsageDetail;
};

type KimiUsageResponse = {
  usage: KimiUsageDetail;
  limits: KimiWindowLimit[];
};

function parseKimiUsageDetail(
  detail: KimiUsageDetail,
  name: string,
): ModelBalanceQuotaWire {
  const limit = parseBalanceValue(detail.limit);
  const remaining = parseBalanceValue(detail.remaining);
  const used =
    detail.used === undefined
      ? Math.max(0, limit - remaining)
      : parseBalanceValue(detail.used);

  return {
    name,
    limit,
    used,
    remaining,
    reset_at:
      typeof detail.resetTime === "string" ? detail.resetTime : undefined,
  };
}

function isKimiUsageDetail(value: unknown): value is KimiUsageDetail {
  const item = assertObject(value);
  const hasValidUsed =
    item.used === undefined ||
    typeof item.used === "string" ||
    typeof item.used === "number";
  return (
    (typeof item.limit === "string" || typeof item.limit === "number") &&
    hasValidUsed &&
    (typeof item.remaining === "string" || typeof item.remaining === "number")
  );
}

function isKimiWindowLimit(value: unknown): value is KimiWindowLimit {
  const item = assertObject(value);
  if (!isKimiUsageDetail(item.detail)) return false;
  const windowObj = assertObject(item.window);
  return (
    typeof windowObj.duration === "number" &&
    typeof windowObj.timeUnit === "string"
  );
}

function isKimiUsageResponse(value: unknown): value is KimiUsageResponse {
  const item = assertObject(value);
  if (!isKimiUsageDetail(item.usage)) return false;
  if (!Array.isArray(item.limits)) return false;
  return item.limits.every(isKimiWindowLimit);
}

export async function fetchKimiCodingBalance(
  baseURL: string,
  apiKey: string,
  signal?: AbortSignal,
): Promise<ModelBalanceDetailWire> {
  const normalizedBaseURL = trimTrailingSlash(baseURL);
  const response = await fetchWithAuth(
    `${normalizedBaseURL}/usages`,
    apiKey,
    signal,
  );
  await assertOk(response);

  const body = await response.json();
  if (!isKimiUsageResponse(body)) {
    throw new BalanceFetchError("Kimi 额度响应格式无效");
  }

  const quotas: ModelBalanceQuotaWire[] = [
    parseKimiUsageDetail(body.usage, "本周请求"),
  ];

  const fiveHourLimit = body.limits.find(
    (limit) =>
      limit.window.duration === 300 &&
      limit.window.timeUnit === "TIME_UNIT_MINUTE",
  );
  if (fiveHourLimit) {
    quotas.push(parseKimiUsageDetail(fiveHourLimit.detail, "5 小时请求"));
  }

  return { kind: "quota", quotas };
}

export async function fetchKimiBalance(
  baseURL: string,
  apiKey: string,
  signal?: AbortSignal,
): Promise<ModelBalanceDetailWire> {
  const response = await fetchWithAuth(
    `${trimTrailingSlash(baseURL)}/users/me/balance`,
    apiKey,
    signal,
  );
  await assertOk(response);
  const body = assertObject(await response.json());
  const data = assertObject(body.data);
  if (body.status !== true || parseBalanceValue(data.available_balance) < 0) {
    throw new BalanceFetchError("Kimi 余额响应格式无效");
  }
  for (const field of ["available_balance", "voucher_balance", "cash_balance"] as const) {
    if (typeof data[field] !== "number" && typeof data[field] !== "string") {
      throw new BalanceFetchError("Kimi 余额响应格式无效");
    }
  }
  return {
    kind: "amount",
    amounts: [{
      currency: "CNY",
      total: parseBalanceValue(data.available_balance),
      granted: parseBalanceValue(data.voucher_balance),
      topped_up: parseBalanceValue(data.cash_balance),
    }],
  };
}

function nestedNumber(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : typeof value === "string" && Number.isFinite(Number(value))
      ? Number(value)
      : 0;
}

function collectResults(body: unknown): Record<string, unknown>[] {
  const data = Array.isArray(assertObject(body).data) ? assertObject(body).data as unknown[] : [];
  return data.flatMap((bucket) => {
    const value = assertObject(bucket);
    return Array.isArray(value.results)
      ? value.results.map(assertObject)
      : [];
  });
}

async function fetchPagedResults(
  initialURL: string,
  headers: Record<string, string>,
  signal?: AbortSignal,
): Promise<Record<string, unknown>[]> {
  let url: string | undefined = initialURL;
  const results: Record<string, unknown>[] = [];
  for (let pageIndex = 0; url && pageIndex < 20; pageIndex += 1) {
    const response = await fetch(url, { headers, signal });
    await assertOk(response);
    const body = await response.json();
    results.push(...collectResults(body));
    const record = assertObject(body);
    const nextPage = typeof record.next_page === "string" ? record.next_page : "";
    if (!nextPage) break;
    const nextURL: URL = new URL(url);
    nextURL.searchParams.set("page", nextPage);
    url = nextURL.toString();
  }
  return results;
}

export async function fetchOpenAIOrganizationUsage(
  adminKey: string,
  signal?: AbortSignal,
): Promise<ModelBalanceDetailWire> {
  const endTime = Math.floor(Date.now() / 1000);
  const startTime = endTime - 7 * 24 * 60 * 60;
  const headers = { Accept: "application/json", Authorization: `Bearer ${adminKey}` };
  const [usageResults, costResults] = await Promise.all([
    fetchPagedResults(`https://api.openai.com/v1/organization/usage/completions?start_time=${startTime}&end_time=${endTime}&bucket_width=1d`, headers, signal),
    fetchPagedResults(`https://api.openai.com/v1/organization/costs?start_time=${startTime}&end_time=${endTime}&bucket_width=1d`, headers, signal),
  ]);
  const sum = (field: string) => usageResults.reduce((total, item) => total + nestedNumber(item[field]), 0);
  const cost = costResults.reduce((total, item) => {
    const amount = item.amount;
    return total + (amount && typeof amount === "object" ? nestedNumber((amount as Record<string, unknown>).value) : 0);
  }, 0);
  return {
    kind: "usage",
    metrics: [
      { name: "近 7 天输入", value: sum("input_tokens"), unit: "tokens" },
      { name: "近 7 天缓存输入", value: sum("input_cached_tokens"), unit: "tokens" },
      { name: "近 7 天输出", value: sum("output_tokens"), unit: "tokens" },
      { name: "近 7 天请求", value: sum("num_model_requests"), unit: "requests" },
      { name: "近 7 天成本", value: cost, unit: "usd" },
    ],
    console_url: "https://platform.openai.com/usage",
  };
}

export async function fetchAnthropicOrganizationUsage(
  adminKey: string,
  signal?: AbortSignal,
): Promise<ModelBalanceDetailWire> {
  const endingAt = new Date();
  const startingAt = new Date(endingAt.getTime() - 7 * 24 * 60 * 60 * 1000);
  const query = `starting_at=${encodeURIComponent(startingAt.toISOString())}&ending_at=${encodeURIComponent(endingAt.toISOString())}&bucket_width=1d`;
  const headers = { Accept: "application/json", "x-api-key": adminKey, "anthropic-version": "2023-06-01" };
  const [usageResults, costResults] = await Promise.all([
    fetchPagedResults(`https://api.anthropic.com/v1/organizations/usage_report/messages?${query}`, headers, signal),
    fetchPagedResults(`https://api.anthropic.com/v1/organizations/cost_report?${query}`, headers, signal),
  ]);
  const sum = (field: string) => usageResults.reduce((total, item) => total + nestedNumber(item[field]), 0);
  const cacheWrite = usageResults.reduce((total, item) => {
    const cache = item.cache_creation;
    if (!cache || typeof cache !== "object") return total;
    const record = cache as Record<string, unknown>;
    return total + nestedNumber(record.ephemeral_5m_input_tokens) + nestedNumber(record.ephemeral_1h_input_tokens);
  }, 0);
  const costCents = costResults.reduce((total, item) => total + nestedNumber(item.amount), 0);
  return {
    kind: "usage",
    metrics: [
      { name: "近 7 天非缓存输入", value: sum("uncached_input_tokens"), unit: "tokens" },
      { name: "近 7 天缓存读取", value: sum("cache_read_input_tokens"), unit: "tokens" },
      { name: "近 7 天缓存写入", value: cacheWrite, unit: "tokens" },
      { name: "近 7 天输出", value: sum("output_tokens"), unit: "tokens" },
      { name: "近 7 天成本", value: costCents / 100, unit: "usd" },
    ],
    console_url: "https://console.anthropic.com/settings/usage",
  };
}

export { BalanceFetchError };
