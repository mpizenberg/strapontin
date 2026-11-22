import { defineConfig } from "vite";
import solidPlugin from "@solidjs/vite-plugin";

export default defineConfig({
  plugins: [solidPlugin()],
  server: {
    proxy: { "/api": "http://localhost:8080" },
  },
  // dist/.gitkeep stays so the Go embed compiles before the first build.
  build: { target: "es2022", emptyOutDir: false },
});
