import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Em dev, o painel roda em :5173 e repassa a API para o Go em :8080 (mesma origem para o cookie).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: { "/admin": "http://localhost:8080", "/v1": "http://localhost:8080" },
  },
});
