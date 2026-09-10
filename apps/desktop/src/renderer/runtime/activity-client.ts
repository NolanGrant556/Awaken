import type { AwakenBridge } from "../../shared/runtime";
import { runtimeRequestIssue, unwrapRuntimeRequest } from "../../shared/runtime-request";
import type { SubscribeHandlers } from "./client";
import { reconnectingSubscription } from "./reconnecting-subscription";

export function threadActivitySubscriber(bridge: AwakenBridge) {
  const subscribe = bridge.subscribeThreadActivity;
  if (!subscribe) return undefined;
  return (threadId: string, handlers: SubscribeHandlers, afterCursor = 0) => reconnectingSubscription({
    connect: (callbacks, cursor) => unwrapRuntimeRequest(subscribe(threadId, callbacks, cursor)),
    disconnect: id => bridge.unsubscribeRun(id),
    handlers,
    cursor: afterCursor,
    eventCursor: event => event.streamCursor,
    terminalError: error => runtimeRequestIssue(error).code === "ACTIVITY_CURSOR_EXPIRED" ? "ACTIVITY_CURSOR_EXPIRED" : undefined,
  });
}
