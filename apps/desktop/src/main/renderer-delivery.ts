export type RendererMessageTarget = {
  isDestroyed: () => boolean;
  send: (channel: string, payload: unknown) => void;
};

export function safelySendToRenderer(
  target: RendererMessageTarget | undefined,
  channel: string,
  payload: unknown,
): boolean {
  if (!target || target.isDestroyed()) {
    return false;
  }

  try {
    target.send(channel, payload);
    return true;
  } catch {
    // A WebContents can still exist after its main frame has been disposed.
    // Treat delivery as unavailable so the stream owner can stop immediately.
    return false;
  }
}

export function shouldRecoverMainRenderer(
  reason: string,
  recoveryAttempts: number,
  maxRecoveryAttempts = 1,
): boolean {
  return reason !== "clean-exit" && recoveryAttempts < maxRecoveryAttempts;
}
