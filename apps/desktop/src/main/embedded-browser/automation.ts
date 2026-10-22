import { setTimeout as delay } from "node:timers/promises";
import { randomUUID } from "node:crypto";
import { BrowserControlGate, type BrowserControl, type BrowserIdentity } from "./control.js";
import {
  type EmbeddedBrowserActionKind,
  type EmbeddedBrowserState,
  originOf,
  parseNavigable,
} from "../../shared/embedded-browser.js";
import type { EmbeddedBrowserViewController } from "./controller.js";

export type BrowserHostAction =
  | "release"
  | "end_owner"
  | "navigate"
  | "snapshot"
  | "click"
  | "type"
  | "wait"
  | "extract";

export type BrowserHostRequest = BrowserIdentity & {
  action: BrowserHostAction;
  threadId: string;
  url?: string;
  ref?: string;
  text?: string;
  seconds?: number;
  start?: number;
};

export type BrowserAutomationHost = {
  agentControl: BrowserControlGate;
  navigateForAgent(threadId: string, url: string, control?: BrowserControl): Promise<EmbeddedBrowserState>;
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
  requestId?: string;
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
  signal?: AbortSignal,
): Promise<BrowserHostResult> {
  const receipt = { requestId: request?.requestId };
  if (!request?.threadId?.trim()) return { ...receipt, ok: false, error: "thread_id_required" };
  if (![request.runId, request.agentId].every((value) => typeof value === "string" && value.trim())) {
    return { ...receipt, ok: false, error: "browser_identity_required" };
  }
  if (request.action === "release" || request.action === "end_owner") {
    manager.agentControl.release(request, request.action === "end_owner");
    return { ...receipt, ok: true };
  }
  if (![request.turnId, request.requestId].every((value) => typeof value === "string" && value.trim())) {
    return { ...receipt, ok: false, error: "browser_identity_required" };
  }
  if (!["navigate", "snapshot", "extract", "wait", "click", "type"].includes(request.action)) {
    return { ...receipt, ok: false, error: "browser_invalid_action" };
  }
  try {
    const result = await manager.agentControl.run(request, request.action, async (control) => {
      const result = await executeBrowserAction(manager, request, control);
      if (!result.ok) throw new Error(result.error ?? "browser_action_failed");
      return result;
    }, signal);
    return { ...result, ...receipt };
  } catch (error) {
    return { ...receipt, ok: false, error: error instanceof Error ? error.message : "browser_action_failed" };
  }
}

async function executeBrowserAction(manager: BrowserAutomationHost, request: BrowserHostRequest, control: BrowserControl): Promise<BrowserHostResult> {
  const threadId = request.threadId?.trim();
  if (!threadId) return { ok: false, error: "thread_id_required" };

  if (request.action === "navigate") {
    const url = parseNavigable(request.url ?? "");
    if (!url) return { ok: false, error: "unsupported_scheme" };
    control.snapshotToken = undefined;
    const state = await manager.navigateForAgent(threadId, url, control);
    control.assertCurrent();
    control.controller = manager.viewForAction(threadId, "observe") ?? undefined;
    if (!control.controller) throw new Error("browser_view_not_visible");
    control.documentRevision = control.controller.documentRevision;
    return { ok: true, url: state.url, title: state.title };
  }

  const kind = request.action === "click" || request.action === "type" ? "mutate" : "observe";
  const controller = manager.viewForAction(threadId, kind);
  if (!controller) {
    return { ok: false, error: kind === "mutate" ? "browser_view_not_visible" : "browser_not_shown" };
  }

  if (control.documentRevision !== controller.documentRevision) throw new Error("browser_navigation_interrupted");
  if (controller !== control.controller) throw new Error("browser_control_lost");
  control.assertCurrent();
  const revision = controller.documentRevision;
  const assertDocument = () => {
    control.assertCurrent();
    if (manager.viewForAction(threadId, kind) !== controller || controller.documentRevision !== revision) {
      throw new Error("browser_navigation_interrupted");
    }
  };

  if (request.action === "wait") {
    const seconds = Math.min(Math.max(Number(request.seconds) || 1, 0.2), 120);
    try {
      await delay(seconds * 1000, undefined, { signal: control.cancellation.signal });
    } catch {
      control.assertCurrent();
      throw new Error("browser_action_cancelled");
    }
    assertDocument();
    const state = controller.state();
    return { ok: true, url: state.url, title: state.title };
  }

  if (request.action === "snapshot") {
    const token = randomUUID();
    const snapshot = await controller.executeJavaScript<{
      url: string;
      title: string;
      items: string[];
    }>(`globalThis.__awakenSnapshotToken = ${JSON.stringify(token)}; ${SNAPSHOT_SCRIPT}`);
    assertDocument();
    control.snapshotToken = token;
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
    assertDocument();
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

  if (!control.snapshotToken) throw new Error("browser_snapshot_expired");
  const snapshotGuard = `if (globalThis.__awakenSnapshotToken !== ${JSON.stringify(control.snapshotToken)}) throw new Error("browser_snapshot_expired");`;
  const ref = normalizeRef(request.ref ?? "");
  if (!ref) return { ok: false, error: "element_ref_required" };
  if (!originStillAdmitted(controller)) {
    return { ok: false, error: "origin_lease_expired" };
  }

  if (request.action === "click") {
    const clicked = await controller.executeJavaScript<boolean>(
      `(() => { ${snapshotGuard} const node = document.querySelector('[data-awaken-ref="${ref}"]'); if (!(node instanceof HTMLElement)) return false; node.click(); return true; })()`,
    );
    control.assertCurrent();
    if (!clicked) return { ok: false, error: "element_not_found" };
    return { ok: true, url: controller.currentURL() };
  }

  const text = request.text ?? "";
  const typed = await controller.executeJavaScript<boolean>(
    `(() => {
      ${snapshotGuard}
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
  assertDocument();
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
