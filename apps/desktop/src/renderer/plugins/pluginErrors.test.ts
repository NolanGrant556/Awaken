import { describe, expect, it } from "vitest";
import { pluginErrorCode, pluginErrorMessage } from "./pluginErrors";

describe("pluginErrorMessage", () => {
  it("maps every Plugin API error domain without exposing IPC or paths", () => {
    const codes = [
      "PLUGIN_NOT_FOUND",
      "PLUGIN_ALREADY_EXISTS",
      "PLUGIN_DIGEST_CHANGED",
      "PLUGIN_RECORDED_DIGEST_CHANGED",
      "PLUGIN_TRASH_UNAVAILABLE",
      "RUNTIME_GENERATION_IN_USE",
      "COMPONENT_NOT_SWITCHABLE",
      "MCP_TOOL_NOT_FOUND",
      "MCP_SOURCE_INVALID",
      "PACKAGE_NO_USABLE_COMPONENTS",
      "PLUGIN_DISABLED",
      "PLUGIN_INVALID",
      "PLUGIN_STATE_CORRUPT",
      "PLUGIN_REQUEST_FAILED",
      "PLUGIN_SOURCE_CHANGED",
      "PLUGIN_SKILL_ID_UNAVAILABLE",
      "VARIABLE_REQUIRED",
      "INVALID_REQUEST",
    ];

    for (const code of codes) {
      const error = new Error(
        `Error invoking remote method 'awaken:plugin-test': Error: ${code}: open /Users/test/private/plugin.json`,
      );
      const message = pluginErrorMessage(error);
      expect(pluginErrorCode(error)).toBe(code);
      expect(message).toContain(`（${code}）`);
      expect(message).not.toContain("invoking remote method");
      expect(message).not.toContain("/Users/");
    }
  });

  it("uses source-edit wording for invalid MCP text", () => {
    expect(pluginErrorMessage(new Error("MCP_SOURCE_INVALID: mcp.json is not valid JSON"))).toBe(
      "MCP 源文件格式不符合要求（MCP_SOURCE_INVALID）",
    );
  });

  it("sanitizes an unknown error and keeps a useful fallback", () => {
    const message = pluginErrorMessage(
      new Error("Error invoking remote method 'awaken:plugin-test': Error: open /Users/test/private/plugin.json: permission denied"),
    );
    expect(message).toContain("本地路径");
    expect(message).toContain("permission denied");
    expect(message).not.toContain("invoking remote method");
    expect(message).not.toContain("/Users/");
  });
});
