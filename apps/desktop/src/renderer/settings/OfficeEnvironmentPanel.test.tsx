// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { RuntimeClient } from "../runtime/client";
import type { OfficeEnvironmentProgress, OfficeEnvironmentResult } from "../../shared/office-environment";
vi.mock("@awaken/design-system", async () => import("../../../../../Awaken Design System/src/components/AwakenOfficeEnvironment"));
import { OfficeEnvironmentPanel } from "./OfficeEnvironmentPanel";
let root: Root, container: HTMLDivElement;
beforeEach(() => { (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true; container=document.createElement("div");document.body.append(container);root=createRoot(container); });
afterEach(async () => { await act(async()=>root.unmount());container.remove(); });
const ready = {state:"ready",required_version:"1",installed_version:"1",instance_id:"old",download_bytes:1024,expanded_bytes:2048,can_prepare:true,can_repair:true,message:"办公环境已就绪",runtimes:[{id:"python",state:"ready",version:"3.12.14"},{id:"node",state:"ready",version:"22.15.0"}]};
function setup(status = {...ready,state:"damaged",runtimes:[ready.runtimes[0],{...ready.runtimes[1],state:"damaged"}]}) {
 let emit: (event:OfficeEnvironmentProgress)=>void = ()=>{};
 let finish:(result:OfficeEnvironmentResult)=>void = ()=>{};
 const client = {getOfficeEnvironment:vi.fn(async()=>status),prepareOfficeEnvironment:vi.fn(()=>new Promise<OfficeEnvironmentResult>(resolve=>{finish=resolve})),cancelOfficeEnvironment:vi.fn(async()=>{}),onOfficeEnvironmentProgress:vi.fn((callback)=>{emit=callback;return ()=>{};})};
 return {client,emit:(event:OfficeEnvironmentProgress)=>emit(event),finish:(result:OfficeEnvironmentResult)=>finish(result)};
}
function button(label:string) {return [...container.querySelectorAll("button")].find(b=>b.textContent===label)!;}
it("status never installs; repair consumes progress scoped to its waiter, and cancelled status never shows ready success",async()=>{
 const test=setup();await act(async()=>root.render(<OfficeEnvironmentPanel runtimeClient={test.client as unknown as RuntimeClient}/>));
 expect(test.client.prepareOfficeEnvironment).not.toHaveBeenCalled();
 await act(async()=>button("修复").click());
 const req=(test.client.prepareOfficeEnvironment.mock.calls[0] as unknown as [any])[0];
 expect(req).toMatchObject({action:"repair",expectedInstanceId:"old"});
 await act(async()=>test.emit({requestId:"foreign",operation:{operation_id:"x",attempt_id:"y",state:"downloading",message:"外部任务",downloaded_bytes:0,total_bytes:2048,updated_at:""}}));
 expect(container.textContent).not.toContain("外部任务");
 await act(async()=>test.emit({requestId:req.requestId,operation:{operation_id:"x",attempt_id:"y",state:"downloading",message:"正在下载",downloaded_bytes:1024*1024,total_bytes:2*1024*1024,updated_at:""}}));
 expect(container.textContent).toContain("1.0 MiB / 2.0 MiB");
 expect(container.querySelector("progress")?.value).toBe(50);
 await act(async()=>button("取消").click());
 expect(test.client.cancelOfficeEnvironment).toHaveBeenCalledWith(req.requestId);
 await act(async()=>test.finish({status:"cancelled"}));
 expect(container.textContent).toContain("已取消等待");
 expect(container.textContent).not.toContain("回到原会话后可以继续任务");
});
it("closing the panel cancels its waiter and ignores late results",async()=>{
 const test=setup();await act(async()=>root.render(<OfficeEnvironmentPanel runtimeClient={test.client as unknown as RuntimeClient}/>));
 await act(async()=>button("修复").click());
 const req=(test.client.prepareOfficeEnvironment.mock.calls[0] as unknown as [any])[0];
 await act(async()=>root.render(null));
 expect(test.client.cancelOfficeEnvironment).toHaveBeenCalledWith(req.requestId);
 await act(async()=>test.finish({status:"ready"}));
 expect(test.client.getOfficeEnvironment).toHaveBeenCalledTimes(1);
});
it("failed repair leaves an actionable repair button and performs no automatic task replay",async()=>{
 const test=setup();await act(async()=>root.render(<OfficeEnvironmentPanel runtimeClient={test.client as unknown as RuntimeClient}/>));
 await act(async()=>button("修复").click());
 await act(async()=>test.finish({status:"failed",message:"校验失败，请重试"}));
 expect(container.querySelector('[role="alert"]')?.textContent).toContain("校验失败");
 expect(button("修复").disabled).toBe(false);
 expect(test.client.prepareOfficeEnvironment).toHaveBeenCalledTimes(1);
});
it("shows exactly two checked environments and one disabled repair action when healthy",async()=>{
 const test=setup(ready);await act(async()=>root.render(<OfficeEnvironmentPanel runtimeClient={test.client as unknown as RuntimeClient}/>));
 expect(container.querySelectorAll('[data-state="ready"] svg')).toHaveLength(2);
 expect([...container.querySelectorAll("li")].map(row=>row.textContent)).toEqual(["Python 环境3.12.14正常","Node.js 环境22.15.0正常"]);
 expect(container.querySelectorAll("button")).toHaveLength(1);
 expect(button("修复").disabled).toBe(true);
 expect(container.textContent).not.toContain("文档处理组件");
 expect(test.client.prepareOfficeEnvironment).not.toHaveBeenCalled();
});
it("a missing environment uses prepare through the same repair button, without claiming ready",async()=>{
 const test=setup({...ready,state:"missing",installed_version:"",instance_id:"",runtimes:ready.runtimes.map(row=>({...row,state:"missing",version:""}))});
 await act(async()=>root.render(<OfficeEnvironmentPanel runtimeClient={test.client as unknown as RuntimeClient}/>));
 expect(container.querySelector('[data-state="ready"]')).toBeNull();
 expect(container.textContent).toContain("需要安装");
 await act(async()=>button("修复").click());
 expect(test.client.prepareOfficeEnvironment).toHaveBeenCalledWith(expect.objectContaining({action:"prepare"}));
 expect((test.client.prepareOfficeEnvironment.mock.calls[0] as unknown as [any])[0]).not.toHaveProperty("expectedInstanceId");
});
it("an old status response without per-runtime checks never paints green checks",async()=>{
 const test=setup({...ready,runtimes:[]});
 await act(async()=>root.render(<OfficeEnvironmentPanel runtimeClient={test.client as unknown as RuntimeClient}/>));
 expect(container.querySelector('[data-state="ready"]')).toBeNull();
 expect(button("修复").disabled).toBe(true);
});

it("shows the actionable status error without the Electron IPC wrapper", async () => {
 const test = setup();
 test.client.getOfficeEnvironment.mockRejectedValue(new Error("Error invoking remote method 'awaken:office-environment-status': Error: 本地服务尚未就绪，请稍后重试"));
 await act(async () => root.render(<OfficeEnvironmentPanel runtimeClient={test.client as unknown as RuntimeClient}/>));
 expect(container.textContent).toContain("本地服务尚未就绪，请稍后重试");
 expect(container.textContent).not.toContain("Error invoking remote method");
});
