import { randomBytes } from "node:crypto";
import { createServer, type IncomingMessage, type Server as HttpServer, type ServerResponse } from "node:http";
import type { EmbeddedBrowserViewManager } from "./manager.js";
import { BROWSER_HOST_TOKEN_BYTES } from "./options.js";
import { runBrowserAutomation, type BrowserHostRequest } from "./automation.js";

export type BrowserHostRegistration = {
  base_url: string;
  token: string;
};

export class EmbeddedBrowserHost {
  private server: HttpServer | null = null;
  private token = "";
  private baseUrl = "";

  constructor(private readonly manager: EmbeddedBrowserViewManager) {}

  get registration(): BrowserHostRegistration | null {
    if (!this.baseUrl || !this.token) return null;
    return { base_url: this.baseUrl, token: this.token };
  }

  async start(): Promise<BrowserHostRegistration> {
    if (this.registration) return this.registration;
    this.token = randomBytes(BROWSER_HOST_TOKEN_BYTES).toString("base64url");
    this.server = createServer((request, response) => {
      void this.handle(request, response);
    });
    await new Promise<void>((resolve, reject) => {
      this.server?.once("error", reject);
      this.server?.listen(0, "127.0.0.1", () => resolve());
    });
    const address = this.server.address();
    if (!address || typeof address === "string") {
      throw new Error("embedded browser host did not bind a TCP port");
    }
    this.baseUrl = `http://127.0.0.1:${address.port}`;
    return this.registration!;
  }

  async stop(): Promise<void> {
    const server = this.server;
    this.server = null;
    this.baseUrl = "";
    this.token = "";
    if (!server) return;
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }

  private async handle(request: IncomingMessage, response: ServerResponse): Promise<void> {
    try {
      if (!this.authorized(request)) {
        this.write(response, 401, { ok: false, error: "unauthorized" });
        return;
      }
      if (request.method === "GET" && request.url === "/health") {
        this.write(response, 200, { ok: true });
        return;
      }
      if (request.method !== "POST" || request.url !== "/v1/actions") {
        this.write(response, 404, { ok: false, error: "not_found" });
        return;
      }
      const body = (await readJson(request)) as BrowserHostRequest;
      const result = await runBrowserAutomation(this.manager, body);
      this.write(response, result.ok ? 200 : 409, result);
    } catch (error) {
      this.write(response, 500, {
        ok: false,
        error: error instanceof Error ? error.message : String(error),
      });
    }
  }

  private authorized(request: IncomingMessage): boolean {
    const header = request.headers.authorization ?? "";
    const prefix = "Bearer ";
    if (!header.startsWith(prefix) || !this.token) return false;
    return header.slice(prefix.length) === this.token;
  }

  private write(response: ServerResponse, status: number, body: unknown): void {
    const payload = Buffer.from(JSON.stringify(body));
    response.writeHead(status, {
      "Content-Type": "application/json",
      "Content-Length": payload.length,
    });
    response.end(payload);
  }
}

function readJson(request: IncomingMessage): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    request.on("data", (chunk) => chunks.push(Buffer.from(chunk)));
    request.on("end", () => {
      if (chunks.length === 0) {
        resolve({});
        return;
      }
      try {
        resolve(JSON.parse(Buffer.concat(chunks).toString("utf8")));
      } catch (error) {
        reject(error);
      }
    });
    request.on("error", reject);
  });
}
