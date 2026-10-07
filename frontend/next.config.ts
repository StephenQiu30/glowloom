import type { NextConfig } from "next";
import { loadEnvConfig } from "@next/env";
import { resolve } from "node:path";

loadEnvConfig(resolve(process.cwd(), ".."));
const addressError = "LV_API_BASE_URL 必须是无凭据的 HTTP(S) 服务根地址。";
let apiBase: URL;
try {
  apiBase = new URL(process.env.LV_API_BASE_URL ?? "http://127.0.0.1:8080");
} catch {
  throw new Error(addressError);
}
if (
  !["http:", "https:"].includes(apiBase.protocol) ||
  apiBase.username ||
  apiBase.password ||
  apiBase.search ||
  apiBase.hash ||
  apiBase.pathname !== "/"
) {
  throw new Error(addressError);
}

const nextConfig: NextConfig = {
  // Self-contained server for the container image (OPS-01 §3).
  output: "standalone",
  poweredByHeader: false,
  // Browser access uses loopback while Docker binds the dev server to 0.0.0.0.
  allowedDevOrigins: ["127.0.0.1"],
  async rewrites() {
    return {
      // Browser API calls use the application-owned Go service through one origin.
      fallback: [
        { source: "/api/:path*", destination: `${apiBase.origin}/api/:path*` },
      ],
    };
  },
};

export default nextConfig;
