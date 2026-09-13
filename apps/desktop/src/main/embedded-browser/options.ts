import type { WebPreferences } from "electron";
import { EMBEDDED_BROWSER_PARTITION } from "../../shared/embedded-browser";

export const BROWSER_HOST_TOKEN_BYTES = 24;

export function browserViewWebPreferences(): WebPreferences {
  return {
    partition: EMBEDDED_BROWSER_PARTITION,
    sandbox: true,
    contextIsolation: true,
    nodeIntegration: false,
    webSecurity: true,
  };
}
