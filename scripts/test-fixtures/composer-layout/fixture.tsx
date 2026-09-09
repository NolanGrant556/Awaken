import "../../../Awaken Design System/src/tokens/tokens.css";
import React from "react";
import { createRoot } from "react-dom/client";
import { flushSync } from "react-dom";
import { AwakenComposerBar } from "../../../Awaken Design System/src/components/AwakenComposerBar/AwakenComposerBar";
import { AwakenUserPromptBlock } from "../../../Awaken Design System/src/components/AwakenUserPromptBlock/AwakenUserPromptBlock";
import { AwakenTaskWelcome } from "../../../Awaken Design System/src/components/AwakenTaskWelcome/AwakenTaskWelcome";
import { DesktopStartupStatus } from "../../../apps/desktop/src/renderer/DesktopStartupStatus";
import "../../../apps/desktop/src/renderer/styles/workbench.css";

const root = createRoot(document.getElementById("root")!);
const noop = () => {};
const tokens = [
  { id: "skill", type: "skill", label: "ppt", leadingText: "文字 ABC123 " },
  { id: "plugin", type: "plugin", label: "插件", leadingText: " 然后 " },
  { id: "computer-use", type: "capability", label: "Computer Use", leadingText: " 以及 " },
];
const settle = () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
async function render(node: React.ReactNode) {
  flushSync(() => root.render(node));
  await settle();
}
function assert(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message);
}
function textRect(node: Node) {
  const range = document.createRange();
  range.selectNodeContents(node);
  return range.getBoundingClientRect();
}
function checkTokens(prefix: string) {
  const nodes = Array.from(document.querySelectorAll<HTMLElement>(`.${prefix}-token`));
  assert(nodes.length === 3, "all token types must remain visible");
  return nodes.map((token) => {
    const label = token.querySelector<HTMLElement>(`.${prefix}-token-label`)!;
    const icon = token.querySelector<HTMLElement>(`.${prefix}-token-icon`)!;
    const before = token.previousSibling!;
    const plain = textRect(before);
    const text = textRect(label);
    const box = token.getBoundingClientRect();
    const iconBox = icon.getBoundingClientRect();
    assert(Math.abs(plain.y - text.y) <= 0.5, `${token.textContent}: text baseline differs by ${text.y - plain.y}px`);
    assert(getComputedStyle(label).fontSize === "14px", "token text must inherit body size");
    assert(Math.abs(iconBox.y + iconBox.height / 2 - box.y - box.height / 2) <= 0.5, "icon must center on its text line");
    assert(iconBox.width === 16 && iconBox.height === 16, "icon must keep its shared 16px size");
    return { label: label.textContent, textTop: text.y, bodyTop: plain.y, iconHeight: iconBox.height };
  });
}

window.runComposerLayoutRegression = async () => {
  const results: Array<{ name: string; passed: boolean; details?: unknown; error?: string }> = [];
  async function check(name: string, test: () => Promise<unknown>) {
    try { results.push({ name, passed: true, details: await test() }); }
    catch (error) { results.push({ name, passed: false, error: String(error) }); }
  }
  for (const variant of ["center", "dock", "embedded"] as const) {
    await check(`${variant}: mixed text and tokens share a baseline`, async () => {
      await render(<div style={{ width: 760, margin: 40 }}><AwakenComposerBar variant={variant} value=" 后续正文" tokens={tokens} onChange={noop} onSubmit={noop} /></div>);
      return checkTokens("composer-bar");
    });
  }
  await check("prompt bubble preserves the same token baseline", async () => {
    await render(<div style={{ width: 760, margin: 40 }}><AwakenUserPromptBlock text=" 后续正文" tokens={tokens} /></div>);
    return checkTokens("execution-prompt-bubble");
  });
  await check("restart notice stays above the brand without moving the composer", async () => {
    const welcome = (needsRestart: boolean) => <div style={{ width: 760, margin: "120px auto" }}>
      <div className="desktop-task-welcome"><DesktopStartupStatus starting={false} needsRestart={needsRestart} /><AwakenTaskWelcome /></div>
      <AwakenComposerBar variant="center" value="" onChange={noop} onSubmit={noop} />
    </div>;
    await render(welcome(false));
    const before = document.querySelector(".composer-bar")!.getBoundingClientRect();
    await render(welcome(true));
    const notice = document.querySelector<HTMLElement>('[role="alert"]');
    assert(notice, "restart error must remain visible");
    const brand = document.querySelector(".aw-task-welcome__logo")!.getBoundingClientRect();
    const after = document.querySelector(".composer-bar")!.getBoundingClientRect();
    assert(notice.getBoundingClientRect().bottom < brand.top, "restart notice must be above the logo");
    assert(before.y === after.y, "restart notice must not move the input");
    assert(getComputedStyle(notice).backgroundColor === "rgba(0, 0, 0, 0)", "notice must be plain text");
    return { noticeBottom: notice.getBoundingClientRect().bottom, logoTop: brand.top, composerTop: after.top };
  });
  for (const left of [20, 260]) {
    await check(`preview respects dialog clipping at offset ${left}`, async () => {
      await render(<div id="clip" style={{ position: "absolute", left: 160, top: 80, width: 560, height: 500, overflow: "auto" }}>
        <div style={{ position: "absolute", left, top: 220, width: 280 }}>
          <AwakenComposerBar variant="embedded" value="@" onChange={noop} onSubmit={noop} slashPalette={{ open: true, actions: [{ id: "outlook", type: "plugin", label: "Outlook 邮箱", description: "这是用于验证弹窗边界的说明文字，必须完整留在弹窗可见区域内。" }] }} />
        </div>
      </div>);
      const option = document.querySelector<HTMLElement>('[role="option"]')!;
      option.dispatchEvent(new MouseEvent("mouseover", { bubbles: true }));
      await settle();
      const preview = document.querySelector<HTMLElement>(".composer-bar-slash-preview")!;
      assert(preview && getComputedStyle(preview).visibility === "visible", "hover must show the preview");
      const box = preview.getBoundingClientRect(), clip = document.getElementById("clip")!.getBoundingClientRect();
      assert(box.left >= clip.left + 7.5 && box.right <= clip.right - 7.5, `preview clipped horizontally: ${box.left}..${box.right}, boundary ${clip.left}..${clip.right}`);
      assert(box.top >= clip.top + 7.5 && box.bottom <= clip.bottom - 7.5, "preview clipped vertically");
      const details = { preview: box.toJSON(), clippingBoundary: clip.toJSON() };
      document.querySelector(".composer-bar-slash-list")!.dispatchEvent(new Event("scroll"));
      await settle();
      assert(!document.querySelector(".composer-bar-slash-preview"), "scrolling must clear the old hovered preview");
      return details;
    });
  }
  await check("palette clamps a caret near a scrolling dialog edge", async () => {
    await render(<div id="clip" style={{ position: "absolute", left: 160, top: 80, width: 560, height: 500, overflow: "auto" }}>
      <div style={{ margin: 20, paddingTop: 260 }}>
        <AwakenComposerBar variant="embedded" value={"ABC123 ".repeat(8) + "@"} onChange={noop} onSubmit={noop}
          slashPalette={{ open: true, actions: [{ id: "outlook", type: "plugin", label: "Outlook 邮箱", description: "边界验收" }] }} />
      </div>
    </div>);
    const textarea = document.querySelector("textarea")!;
    textarea.setSelectionRange(textarea.value.length, textarea.value.length);
    textarea.focus();
    textarea.dispatchEvent(new Event("scroll", { bubbles: true }));
    await settle();
    const palette = document.querySelector<HTMLElement>(".composer-bar-slash-palette")!;
    const clip = document.getElementById("clip")!;
    const bounds = clip.getBoundingClientRect(), box = palette.getBoundingClientRect();
    assert(getComputedStyle(palette).visibility === "visible", "palette must become positioned");
    assert(box.left >= bounds.left + 7.5 && box.right <= bounds.right - 7.5, `palette clipped horizontally: ${box.right} > ${bounds.right}`);
    assert(box.top >= bounds.top + 7.5 && box.bottom <= bounds.bottom - 7.5, "palette clipped vertically");
    return { palette: box.toJSON(), boundary: bounds.toJSON() };
  });
  root.unmount();
  return results;
};

declare global { interface Window { runComposerLayoutRegression: () => Promise<unknown>; } }
