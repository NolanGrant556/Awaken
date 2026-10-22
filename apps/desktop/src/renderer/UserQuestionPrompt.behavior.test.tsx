// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { UserQuestionPrompt } from "./UserQuestionPrompt";

(globalThis as Record<string,unknown>).IS_REACT_ACT_ENVIRONMENT = true;
let dispose: (() => void) | undefined;
afterEach(() => {dispose?.();dispose=undefined;});
it("Escape from an answer input cancels once, while composition and inner menus consume it", async () => {
 const host=document.createElement("div");document.body.append(host);const root=createRoot(host);dispose=()=>{act(()=>root.unmount());host.remove();};
 const cancel=vi.fn(); const question={id:"q",questions:[{question:"Choose",options:[{label:"A"},{label:"B"}]}]};
 await act(async()=>root.render(<UserQuestionPrompt question={question} onCancel={cancel} onSubmit={vi.fn()}/>));
 await act(async()=>host.querySelector<HTMLElement>('[role="button"]')!.click());
 const input=host.querySelector("input")!;expect(input).not.toBeNull();
 await act(async()=>input.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true,isComposing:true})));
 expect(cancel).not.toHaveBeenCalled();
 const inner=(event:Event)=>event.preventDefault();input.addEventListener("keydown",inner);
 await act(async()=>input.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true})));
 expect(cancel).not.toHaveBeenCalled();input.removeEventListener("keydown",inner);
 await act(async()=>input.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true})));
 expect(cancel).toHaveBeenCalledTimes(1);
 await act(async()=>root.render(<UserQuestionPrompt question={{...question,withdrawn:true}} onCancel={cancel} onSubmit={vi.fn()}/>));
 expect(host.querySelector("input")).toBeNull();expect(host.querySelector("button")).toBeNull();
 await act(async()=>host.firstElementChild!.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true})));
 expect(cancel).toHaveBeenCalledTimes(1);
});


it("collapses without resolving the question, preserves free text and page, and opens a new question", async () => {
  const host = document.createElement("div"); document.body.append(host);
  const root = createRoot(host); dispose = () => { act(() => root.unmount()); host.remove(); };
  const cancel = vi.fn(), submit = vi.fn();
  const question = { id: "collapse-q", questions: [
    { question: "First", options: [{ label: "A" }], multiSelect: true },
    { question: "Second", options: [{ label: "B" }] },
  ] };
  const render = (id = question.id) => root.render(<UserQuestionPrompt question={{ ...question, id }} onCancel={cancel} onSubmit={submit} />);
  await act(async () => render());
  await act(async () => host.querySelector<HTMLButtonElement>(".user-question-card__option")!.click());
  await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="下一题"]')!.click());
  await act(async () => host.querySelector<HTMLElement>('[role="button"]')!.click());
  const input = host.querySelector("input")!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, "保留草稿");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="收起提问面板"]')!.click());
  expect(host.querySelector<HTMLElement>(".user-question-card__body")!.hidden).toBe(true);
  expect(host.querySelector(".user-question-card__summary")!.textContent).toBe("Second");
  expect(cancel).not.toHaveBeenCalled(); expect(submit).not.toHaveBeenCalled();
  await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="展开提问面板"]')!.click());
  expect(host.querySelector("input")!.value).toBe("保留草稿");
  expect(host.querySelector(".user-question-card__counter")!.textContent).toBe("2 of 2");
  await act(async () => (Array.from(host.querySelectorAll("button")).find(b => b.textContent === "提交回答")!).click());
  expect(submit).toHaveBeenCalledWith("First: A\nSecond: 保留草稿");
  await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="收起提问面板"]')!.click());
  await act(async () => render("next-q"));
  expect(host.querySelector<HTMLElement>(".user-question-card__body")!.hidden).toBe(false);
  expect(host.querySelector("input")).toBeNull();
});
