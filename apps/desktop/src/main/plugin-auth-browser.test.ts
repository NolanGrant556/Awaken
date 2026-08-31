import { describe, expect, it } from "vitest";
import {
  sanitizedPluginAuthSession,
  validatePluginAuthBrowserUrl,
} from "./plugin-auth-browser.js";

describe("plugin auth browser boundary", () => {
  it("accepts public HTTPS and loopback HTTP but rejects unsafe URLs", () => {
    expect(validatePluginAuthBrowserUrl("https://login.example.com/oauth?state=ok")).toBe("https://login.example.com/oauth?state=ok");
    expect(validatePluginAuthBrowserUrl("https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=ok")).toBe("https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=ok");
    expect(validatePluginAuthBrowserUrl("http://127.0.0.1:4567/callback")).toBe("http://127.0.0.1:4567/callback");
    for (const candidate of [
      "http://example.com/oauth",
      "javascript:alert(1)",
      "https://user:pass@example.com/oauth",
      "https://example.com/oauth#token",
      "https://example.com/oauth?access_token=secret",
      "https://192.168.1.10/oauth",
      "https://[fd00::1]/oauth",
    ]) {
      expect(() => validatePluginAuthBrowserUrl(candidate)).toThrow();
    }
  });

  it("strips authorization URLs and device codes from the Renderer DTO", () => {
    const session = sanitizedPluginAuthSession("mail", "account", {
      sessionId: "session",
      status: "authorizing",
      authorizationUrl: "https://login.example.com/oauth",
      userCode: "DEVICE-SECRET",
      displayUserCode: true,
      expiresAt: "2026-08-31T00:00:00Z",
    });
    expect(session).toEqual({
      sessionId: "session",
      pluginName: "mail",
      setupId: "account",
      status: "authorizing",
      expiresAt: "2026-08-31T00:00:00Z",
    });
    expect(JSON.stringify(session)).not.toContain("DEVICE-SECRET");
    expect(JSON.stringify(session)).not.toContain("login.example.com");
  });
});
