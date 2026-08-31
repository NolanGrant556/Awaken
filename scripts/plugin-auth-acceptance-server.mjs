import http from "node:http";

const host = "127.0.0.1";
const port = 37654;
let deviceApproved = false;

function htmlEscape(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

const server = http.createServer(async (request, response) => {
  const target = new URL(request.url ?? "/", `http://${host}:${port}`);
  if (request.method === "GET" && target.pathname === "/authorize") {
    const redirectURI = target.searchParams.get("redirect_uri") ?? "";
    const state = target.searchParams.get("state") ?? "";
    const challenge = target.searchParams.get("code_challenge") ?? "";
    if (!redirectURI.startsWith("http://127.0.0.1:") || !state || !challenge) {
      response.writeHead(400).end("Invalid PKCE authorization request");
      return;
    }
    const callback = new URL(redirectURI);
    callback.searchParams.set("code", "local-approval-code");
    callback.searchParams.set("state", state);
    response.writeHead(200, { "content-type": "text/html; charset=utf-8", "cache-control": "no-store" });
    response.end(`<!doctype html><meta charset="utf-8"><title>Awaken 本地 OAuth 验收</title><style>body{font:16px system-ui;max-width:620px;margin:80px auto;padding:24px}a{display:inline-block;background:#171717;color:#fff;padding:12px 18px;border-radius:9px;text-decoration:none}</style><h1>Awaken 本地 OAuth 验收</h1><p>此页面只连接本机测试服务，不会访问第三方账号。</p><a href="${htmlEscape(callback.toString())}">允许本地验收</a>`);
    return;
  }
  if (request.method === "POST" && target.pathname === "/token") {
    let body = "";
    for await (const chunk of request) body += chunk;
    const form = new URLSearchParams(body);
    const deviceGrant = form.get("grant_type") === "urn:ietf:params:oauth:grant-type:device_code" && form.get("device_code") === "local-device-code";
    if (deviceGrant && !deviceApproved) {
      response.writeHead(400, { "content-type": "application/json", "cache-control": "no-store" }).end(JSON.stringify({ error: "authorization_pending" }));
      return;
    }
    if (deviceGrant) {
      response.writeHead(200, { "content-type": "application/json", "cache-control": "no-store" });
      response.end(JSON.stringify({ access_token: "local-device-access-token", refresh_token: "local-device-refresh-token", token_type: "Bearer", expires_in: 3600 }));
      return;
    }
    const validCode = form.get("grant_type") === "authorization_code" && form.get("code") === "local-approval-code" && form.get("code_verifier");
    const validRefresh = form.get("grant_type") === "refresh_token" && form.get("refresh_token") === "local-refresh-token";
    if (!validCode && !validRefresh) {
      response.writeHead(400, { "content-type": "application/json" }).end(JSON.stringify({ error: "invalid_grant" }));
      return;
    }
    response.writeHead(200, { "content-type": "application/json", "cache-control": "no-store" });
    response.end(JSON.stringify({ access_token: "local-access-token", refresh_token: "local-refresh-token", token_type: "Bearer", expires_in: 3600 }));
    return;
  }
  if (request.method === "POST" && target.pathname === "/device/authorize") {
    deviceApproved = false;
    response.writeHead(200, { "content-type": "application/json", "cache-control": "no-store" });
    response.end(JSON.stringify({
      device_code: "local-device-code",
      user_code: "AWAKEN-LOCAL",
      verification_uri: `http://${host}:${port}/device`,
      expires_in: 120,
      interval: 1
    }));
    return;
  }
  if (request.method === "GET" && target.pathname === "/device") {
    response.writeHead(200, { "content-type": "text/html; charset=utf-8", "cache-control": "no-store" });
    response.end('<!doctype html><meta charset="utf-8"><title>Awaken Device Flow 验收</title><style>body{font:16px system-ui;max-width:620px;margin:80px auto;padding:24px}a{display:inline-block;background:#171717;color:#fff;padding:12px 18px;border-radius:9px;text-decoration:none}</style><h1>Awaken Device Flow 验收</h1><p>确认原生弹窗中的临时代码后，允许本机测试账号。</p><a href="/device/approve">允许本地验收</a>');
    return;
  }
  if (request.method === "GET" && target.pathname === "/device/approve") {
    deviceApproved = true;
    response.writeHead(200, { "content-type": "text/html; charset=utf-8", "cache-control": "no-store" });
    response.end('<!doctype html><meta charset="utf-8"><title>Awaken</title><p>本地 Device Flow 已确认，可以返回 Awaken。</p>');
    return;
  }
  response.writeHead(404).end("Not found");
});

server.listen(port, host, () => {
  process.stdout.write(`Plugin auth acceptance server listening on http://${host}:${port}\n`);
});
