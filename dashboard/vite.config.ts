import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { TanStackRouterVite } from "@tanstack/router-plugin/vite";
import path from "path";

// Override with API_URL=http://localhost:PORT when 8090 is taken.
const API_URL = process.env.API_URL ?? "http://localhost:8090";

export default defineConfig({
  base: "/",
  plugins: [TanStackRouterVite({ quoteStyle: "double" }), react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 3100,
    proxy: {
      "/api": {
        target: API_URL,
        changeOrigin: true,
      },
      // Trailing slash matters: "/r" alone would also proxy /register to the API.
      "/r/": {
        target: API_URL,
        changeOrigin: true,
      },
    },
  },
});
