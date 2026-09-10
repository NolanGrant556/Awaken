import type { AguiEvent } from "./agui";
import type { SubscribeHandlers } from "./client";

type Options = {
  connect: (handlers: SubscribeHandlers, cursor: number) => Promise<string>;
  disconnect: (id: string) => void;
  handlers: SubscribeHandlers;
  cursor?: number;
  eventCursor: (event: AguiEvent) => number | undefined;
  terminalEvent?: (event: AguiEvent) => boolean;
  terminalError: (error: unknown) => string | undefined;
};

// Both transports share ownership, cursor and reconnect mechanics. Their
// terminal/error policies differ: thread activity survives a finished root.
export async function reconnectingSubscription(options: Options): Promise<() => void> {
  let disposed = false;
  let terminal = false;
  let subscriptionId: string | undefined;
  let cursor = options.cursor ?? 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let retries = 0;
  let generation = 0;
  const disconnect = () => {
    const id = subscriptionId;
    subscriptionId = undefined;
    if (id) options.disconnect(id);
  };
  const schedule = (error: unknown, attempt: number) => {
    if (disposed || terminal || timer || attempt !== generation) return;
    generation += 1;
    disconnect();
    const message = options.terminalError(error);
    if (message !== undefined) {
      terminal = true;
      options.handlers.onError?.(message);
      return;
    }
    options.handlers.onReconnect?.();
    timer = setTimeout(() => { timer = undefined; void connect(); }, Math.min(1000 * 2 ** retries++, 10000));
  };
  const connect = async () => {
    if (disposed || terminal) return;
    const attempt = ++generation;
    try {
      const id = await options.connect({
        onEvent: event => {
          if (disposed || terminal || attempt !== generation) return;
          cursor = Math.max(cursor, options.eventCursor(event) ?? 0);
          retries = 0;
          terminal = options.terminalEvent?.(event) ?? false;
          options.handlers.onEvent(event);
        },
        onError: error => schedule(error, attempt),
        onClose: () => {
          if (disposed || attempt !== generation) return;
          if (terminal) {
            generation += 1;
            disconnect();
            options.handlers.onClose?.();
          } else schedule(undefined, attempt);
        },
      }, cursor);
      if (disposed || terminal || attempt !== generation) options.disconnect(id);
      else subscriptionId = id;
    } catch (error) { schedule(error, attempt); }
  };
  await connect();
  return () => {
    disposed = true;
    generation += 1;
    if (timer) clearTimeout(timer);
    disconnect();
  };
}
