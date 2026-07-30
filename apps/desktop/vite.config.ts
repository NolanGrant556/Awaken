import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const runtimeTarget = env.AWAKEN_RUNTIME_URL?.trim() || "http://127.0.0.1:8787";

  return {
    base: "./",
    plugins: [react()],
    server: {
      host: "127.0.0.1",
      port: 5174,
      proxy: {
        "/health": runtimeTarget,
        "/v1": runtimeTarget,
      },
    },
    build: {
      outDir: "dist/renderer",
      emptyOutDir: false,
      rollupOptions: {
        input: {
          main: "index.html",
          petOverlay: "pet-overlay.html",
          computerOverlay: "computer-overlay.html",
        },
      },
    },
  };
});
