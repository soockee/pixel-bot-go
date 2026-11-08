import { defineConfig } from "vite";

export default defineConfig({
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // overlay.html is the see-through window over the game (app/overlay.go).
    rollupOptions: { input: { main: "index.html", overlay: "overlay.html" } },
  },
});
