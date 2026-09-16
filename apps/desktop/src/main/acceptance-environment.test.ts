import os from "node:os";
import path from "node:path";
import { expect, it } from "vitest";
import { acceptanceChildEnvironment } from "./acceptance-environment.js";

it("isolates child account/config paths only for explicitly selected development acceptance", () => {
  const root = path.join(os.tmpdir(), "awaken-explicit-test");
  expect(acceptanceChildEnvironment({ root, dev: true, packaged: true })).toEqual({});
  expect(acceptanceChildEnvironment({ root, dev: false, packaged: false })).toEqual({});
  expect(acceptanceChildEnvironment({ dev: true, packaged: false })).toEqual({});
  const env = acceptanceChildEnvironment({ root, dev: true, packaged: false });
  for (const value of Object.values(env)) expect(path.relative(root, value!)).not.toMatch(/^\.\./);
  expect(env.HOME).toBe(path.join(root, "home"));
  expect(env).not.toHaveProperty("PATH");
  expect(() => acceptanceChildEnvironment({ root: "relative", dev: true, packaged: false })).toThrow("absolute");
});
