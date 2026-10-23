import path from "node:path";
import react from "@vitejs/plugin-react";

const config = {
  root: path.resolve(import.meta.dirname, "../.."),
  plugins: [react()],
  define: { "process.env": "{}" },
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "../../src"),
      "next/navigation": path.resolve(
        import.meta.dirname,
        "canvas-navigation.ts",
      ),
    },
  },
  server: { host: "127.0.0.1", port: 5198, strictPort: true },
};

export default config;
