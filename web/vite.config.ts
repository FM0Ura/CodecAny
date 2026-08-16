import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // O backend Go (cmd/server) roda em 127.0.0.1:8383 por padrão.
      // O proxy padrão do Vite lida nativamente com streaming HTTP, então
      // /api/events (Server-Sent Events via EventSource) funciona sem config extra.
      "/api": {
        target: "http://127.0.0.1:8383",
        changeOrigin: true,
      },
    },
  },
  build: {
    // cmd/server embute este diretório via go:embed (all:webdist).
    outDir: "../cmd/server/webdist",
    emptyOutDir: true,
  },
});
