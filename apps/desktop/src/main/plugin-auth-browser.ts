import type { RuntimePluginAuthSession } from "../shared/runtime.js";

export type CorePluginAuthStartResult = {
  sessionId?: string;
  status: RuntimePluginAuthSession["status"];
  authorizationUrl?: string;
  verificationUrl?: string;
  userCode?: string;
  displayUserCode?: boolean;
  expiresAt?: string;
};

export function validatePluginAuthBrowserUrl(raw: string): string {
  let parsed: URL;
  try {
    parsed = new URL(String(raw ?? "").trim());
  } catch {
    throw new Error("授权地址无效，已阻止打开");
  }
  const hostname = parsed.hostname.toLowerCase().replace(/\.$/, "").replace(/^\[|\]$/g, "");
  const loopback = hostname === "localhost" || hostname === "127.0.0.1" || hostname === "::1";
  if (parsed.username || parsed.password || parsed.hash) {
    throw new Error("授权地址包含不允许的身份或片段信息");
  }
  if (parsed.protocol !== "https:" && !(parsed.protocol === "http:" && loopback)) {
    throw new Error("授权地址必须使用公共 HTTPS 或本机回环 HTTP");
  }
  if (!loopback && isPrivateAddressLiteral(hostname)) {
    throw new Error("授权地址不能指向私网或本机地址");
  }
  for (const key of parsed.searchParams.keys()) {
    if (["access_token", "token", "client_secret", "password", "api_key"].includes(key.toLowerCase())) {
      throw new Error("授权地址包含敏感查询参数");
    }
  }
  return parsed.toString();
}

function isPrivateAddressLiteral(hostname: string): boolean {
  const ipv4 = hostname.split(".").map((part) => Number(part));
  if (ipv4.length === 4 && ipv4.every((part) => Number.isInteger(part) && part >= 0 && part <= 255)) {
    const [first, second] = ipv4;
    return first === 0 || first === 10 || first === 127 || first >= 224
      || (first === 169 && second === 254)
      || (first === 172 && second >= 16 && second <= 31)
      || (first === 192 && second === 168);
  }
  return hostname === "::" || hostname.startsWith("fc") || hostname.startsWith("fd") || hostname.startsWith("fe8") || hostname.startsWith("fe9") || hostname.startsWith("fea") || hostname.startsWith("feb");
}

export function sanitizedPluginAuthSession(
  pluginName: string,
  setupId: string,
  result: CorePluginAuthStartResult,
): RuntimePluginAuthSession {
  return {
    sessionId: result.sessionId ?? "",
    pluginName,
    setupId,
    status: result.status,
    expiresAt: result.expiresAt,
  };
}
