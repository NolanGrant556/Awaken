import type { AguiEvent } from "../../shared/agui";

export function stopMessage(source?: AguiEvent["stopSource"]): string {
  switch (source) {
    case "user": return "你停止了本次运行";
    case "parent": return "主任务已停止此子任务";
    case "shutdown": return "应用关闭，运行已中断";
    case "timeout": return "本次运行已超时";
    default: return "本次运行已中断";
  }
}

export function terminalMessage(event: AguiEvent): string | undefined {
  if (event.code === "RUN_CANCELLED" || event.status === "cancelled" || event.stopSource) {
    return stopMessage(event.stopSource);
  }
  if (event.terminalReason === "model_stream_disconnected") return "模型连接中断，本次运行未能继续";
  if (event.terminalReason === "process_restarted" || event.code === "PROCESS_RESTARTED") return stopMessage();
  return undefined;
}
