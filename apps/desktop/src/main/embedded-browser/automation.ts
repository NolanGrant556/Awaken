import {
  type EmbeddedBrowserActionKind,
  originOf,
  parseNavigable,
} from "../../shared/embedded-browser";
import type { EmbeddedBrowserViewController } from "./controller";

export type BrowserHostAction =
  | "navigate"
  | "snapshot"
  | "click"
  | "type"
  | "wait"
  | "extract";

export type BrowserHostRequest = {
  action: BrowserHostAction;
  threadId: string;
  url?: string;
  ref?: string;
  text?: string;
  seconds?: number;
  start?: number;
};

export type BrowserAutomationHost = {
  ensureVisibleForAgent(threadId: string): Promise<EmbeddedBrowserViewController>;
  viewForAction(
    threadId: string,
    kind: EmbeddedBrowserActionKind,
  ): EmbeddedBrowserViewController | null;
};

const SNAPSHOT_SCRIPT = `(() => {
  const nodes = Array.from(document.querySelectorAll("a, button, input, textarea, select, [role='button'], [role='link'], [role='textbox']"));
  const items = [];
  for (const node of nodes) {
    if (!(node instanceof HTMLElement)) continue;
    const style = window.getComputedStyle(node);
    if (style.display === "none" || style.visibility === "hidden") continue;
    const rect = node.getBoundingClientRect();
    if (rect.width < 2 || rect.height < 2) continue;
    const label = (node.getAttribute("aria-label") || node.innerText || node.getAttribute("placeholder") || node.getAttribute("name") || node.tagName).trim().slice(0, 80);
    const index = items.length + 1;
    node.setAttribute("data-awaken-ref", String(index));
    items.push("[" + index + "] " + node.tagName.toLowerCase() + " " + label.replace(/\\s+/g, " "));
    if (items.length >= 80) break;
  }
  return { url: location.href, title: document.title, items };
})()`;

const EXTRACT_SCRIPT = `(() => {
  const text = document.body ? document.body.innerText : "";
  return { url: location.href, title: document.title, text };
})()`;

export type BrowserHostResult = {
  ok: boolean;
  error?: string;
  url?: string;
  title?: string;
  snapshot?: string;
  text?: string;
  truncated?: boolean;
};

export async function runBrowserAutomation(
  manager: BrowserAutomationHost,
  request: BrowserHostRequest,
): Promise<BrowserHostResult> {
  const threadId = request.threadId?.trim();
  if (!threadId) return { ok: false, error: "thread_id_required" };

  if (request.action === "navigate") {
    const url = parseNavigable(request.url ?? "");
    if (!url) return { ok: false, error: "unsupported_scheme" };
    const controller = await manager.ensureVisibleForAgent(threadId);
    await controller.navigate(url);
    const state = controller.state();
    return { ok: true, url: state.url, title: state.title };
  }

  const kind = request.action === "click" || request.action === "type" ? "mutate" : "observe";
  const controller = manager.viewForAction(threadId, kind);
  if (!controller) {
    return { ok: false, error: kind === "mutate" ? "browser_view_not_visible" : "browser_not_shown" };
  }

  if (request.action === "wait") {
    const seconds = Math.min(Math.max(Number(request.seconds) || 1, 0.2), 120);
    await new Promise((resolve) => setTimeout(resolve, seconds * 1000));
    const state = controller.state();
    return { ok: true, url: state.url, title: state.title };
  }

  if (request.action === "snapshot") {
    const snapshot = await controller.executeJavaScript<{
      url: string;
      title: string;
      items: string[];
    }>(SNAPSHOT_SCRIPT);
    return {
      ok: true,
      url: snapshot.url,
      title: snapshot.title,
      snapshot: snapshot.items.join("\n"),
    };
  }

  if (request.action === "extract") {
    const extracted = await controller.executeJavaScript<{
      url: string;
      title: string;
      text: string;
    }>(EXTRACT_SCRIPT);
    const start = Math.max(0, Math.floor(Number(request.start) || 0));
    const slice = extracted.text.slice(start, start + 16_000);
    return {
      ok: true,
      url: extracted.url,
      title: extracted.title,
      text: slice,
      truncated: start + slice.length < extracted.text.length,
    };
  }

  const ref = normalizeRef(request.ref ?? "");
  if (!ref) return { ok: false, error: "element_ref_required" };
  if (!originStillAdmitted(controller)) {
    return { ok: false, error: "origin_lease_expired" };
  }

  if (request.action === "click") {
    const clicked = await controller.executeJavaScript<boolean>(
      `(() => { const node = document.querySelector('[data-awaken-ref="${ref}"]'); if (!(node instanceof HTMLElement)) return false; node.click(); return true; })()`,
    );
    if (!clicked) return { ok: false, error: "element_not_found" };
    return { ok: true, url: controller.currentURL() };
  }

  const text = request.text ?? "";
  const typed = await controller.executeJavaScript<boolean>(
    `(() => {
      const node = document.querySelector('[data-awaken-ref="${ref}"]');
      if (!(node instanceof HTMLElement)) return false;
      node.focus();
      if (node instanceof HTMLInputElement || node instanceof HTMLTextAreaElement) {
        node.value = ${JSON.stringify(text)};
        node.dispatchEvent(new Event("input", { bubbles: true }));
        return true;
      }
      node.textContent = ${JSON.stringify(text)};
      return true;
    })()`,
  );
  if (!typed) return { ok: false, error: "element_not_found" };
  return { ok: true, url: controller.currentURL() };
}

function normalizeRef(value: string): string {
  const match = /^\s*\[(\d+)\]\s*$/.exec(value) ?? /^\s*(\d+)\s*$/.exec(value);
  return match?.[1] ?? "";
}

function originStillAdmitted(controller: EmbeddedBrowserViewController): boolean {
  const url = controller.currentURL();
  const origin = originOf(url);
  return origin !== "" && controller.originAdmitted(url);
}
