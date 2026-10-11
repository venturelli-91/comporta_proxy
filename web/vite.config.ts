import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// In dev, the dashboard runs on :5173 and proxies the API to Go on :8080 (same origin for the cookie).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: { "/admin": "http://localhost:8080", "/v1": "http://localhost:8080" },
  },
});
