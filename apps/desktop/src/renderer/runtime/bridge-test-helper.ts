import { vi } from "vitest";

/** Model the serialized reply returned by Main, including void results. */
export function mockDesktopReply(implementation: (...args: any[]) => any = () => undefined) {
  return vi.fn(async (...args: any[]) => ({ ok: true as const, value: await implementation(...args) }));
}
