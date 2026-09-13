import type { WebPreferences } from "electron";
import { EMBEDDED_BROWSER_PARTITION } from "../../shared/embedded-browser.js";

export const BROWSER_HOST_TOKEN_BYTES = 24;

export function browserViewWebPreferences(options?: {
  partition?: string;
  allowFile?: boolean;
}): WebPreferences {
  return {
    partition: options?.partition ?? EMBEDDED_BROWSER_PARTITION,
    sandbox: true,
    contextIsolation: true,
    nodeIntegration: false,
    webSecurity: true,
    ...(options?.allowFile ? { allowFileAccessFromFileUrls: true } : {}),
  };
}
