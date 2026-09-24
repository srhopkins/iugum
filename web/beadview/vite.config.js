import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// `npm run dev` forwards /api to a running `iugum beadview`.
// Set BEADVIEW_PORT (default 3849, the beadview default) or
// BEADVIEW_URL (full origin) to point somewhere else.
const target =
  process.env.BEADVIEW_URL ||
  `http://127.0.0.1:${process.env.BEADVIEW_PORT || "3849"}`;

export default defineConfig({
  base: "/",
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": { target, changeOrigin: true },
    },
  },
  resolve: {
    dedupe: ["react", "react-dom"],
  },
  test: {
    environment: "jsdom",
    globals: true,
  },
});
