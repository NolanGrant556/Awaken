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

export { BalanceFetchError };
