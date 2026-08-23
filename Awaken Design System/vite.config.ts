import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";

const externalDependencies = [
  "react",
  "react-dom",
  "react/jsx-runtime",
  "@floating-ui/react",
  "mermaid",
  "react-markdown",
  "rehype-highlight",
  "rehype-sanitize",
  "remark-gfm",
  "thinking-orbs",
];

export default defineConfig(({ mode }) => {
  if (mode === "gallery") {
    return { plugins: [react()] };
  }

  const galleryLibrary = mode === "gallery-library";

  return {
    plugins: [react()],
    build: {
      emptyOutDir: !galleryLibrary,
      lib: {
        entry: resolve(__dirname, galleryLibrary ? "gallery/demos.ts" : "src/index.ts"),
        formats: ["es"],
        fileName: galleryLibrary ? "gallery" : "index",
        cssFileName: galleryLibrary ? "gallery" : "index",
      },
      rollupOptions: {
        external: externalDependencies,
      },
    },
  };
});
