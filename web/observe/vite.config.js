import { copyFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "/",
  plugins: [
    react(),
    {
      name: "copy-homelab-dashboard",
      closeBundle() {
        const src = resolve(__dirname, "homelab-dashboard.json");
        if (existsSync(src)) {
          copyFileSync(src, resolve(__dirname, "dist/homelab-dashboard.json"));
        }
      },
    },
  ],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  resolve: {
    dedupe: ["react", "react-dom"],
  },
});
