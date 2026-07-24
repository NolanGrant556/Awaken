import { defineConfig } from "vitest/config";
import { fileURLToPath } from "node:url";

const designSystemSource = fileURLToPath(
  new URL("../../Awaken Design System/src/index.ts", import.meta.url),
);

export default defineConfig({
  resolve: {
    alias: [
      {
        find: /^@awaken\/design-system$/,
        replacement: designSystemSource,
      },
    ],
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
