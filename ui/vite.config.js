import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// The built UI is embedded into the Go binary (ui/embed.go). In development, /api goes to a local panel.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "src") } },
  build: { outDir: "dist", emptyOutDir: true, chunkSizeWarningLimit: 1500 },
  server: { port: 5190, proxy: { "/api": { target: process.env.LATCHADMIN_URL ?? "http://127.0.0.1:40120", changeOrigin: false } } },
})
