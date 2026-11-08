import { defineConfig } from "vite";

// Two pages: the main window and the selection overlay.
export default defineConfig({
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: { input: { main: "index.html", selection: "selection.html" } },
  },
});
