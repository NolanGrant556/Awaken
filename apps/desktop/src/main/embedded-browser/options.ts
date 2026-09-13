import type { WebPreferences } from "electron";
import {
  EMBEDDED_BROWSER_PARTITION,
  isEmbeddedBrowserPartition,
  normalizeBrowserAddressInput,
  parseNavigable,
} from "../../shared/embedded-browser.js";

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
    ...(options?.allowFile
      ? { allowFileAccessFromFileUrls: true, webSecurity: false }
      : {}),
  };
}

export function authorizeEmbeddedBrowserWebview(
  webPreferences: WebPreferences,
  params: { src?: string; partition?: string },
): boolean {
  const partition = params.partition ?? "";
  const src = params.src ?? "";
  if (!isEmbeddedBrowserPartition(partition)) return false;

  const allowFile = partition !== EMBEDDED_BROWSER_PARTITION;
  if (allowFile) {
    const parsed = normalizeBrowserAddressInput(src);
    if (!parsed.ok || parsed.kind !== "file") return false;
    Object.assign(webPreferences, {
      allowFileAccessFromFileUrls: true,
      webSecurity: false,
    });
  } else if (src && src !== "about:blank" && !parseNavigable(src)) {
    return false;
  }

  delete webPreferences.preload;
  webPreferences.nodeIntegration = false;
  webPreferences.contextIsolation = true;
  webPreferences.sandbox = true;
  webPreferences.webSecurity = !allowFile;
  webPreferences.plugins = false;
  return true;
}
