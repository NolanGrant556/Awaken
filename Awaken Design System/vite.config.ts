import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  build:
    mode === "gallery"
      ? undefined
      : {
          lib: {
            entry: resolve(__dirname, "src/index.ts"),
            formats: ["es"],
            fileName: "index",
          },
          rollupOptions: {
            external: [
              "react",
              "react-dom",
              "react/jsx-runtime",
              "@floating-ui/react",
              "lucide-react",
              "mermaid",
              "react-markdown",
              "rehype-sanitize",
              "remark-gfm",
            ],
          },
        },
}));
