import type { NextConfig } from "next";
import { loadEnvConfig } from "@next/env";
import { resolve } from "node:path";

loadEnvConfig(resolve(process.cwd(), ".."));
const apiBase = new URL(process.env.LV_API_BASE_URL ?? "http://127.0.0.1:8080");
if (
  !["http:", "https:"].includes(apiBase.protocol) ||
  apiBase.username ||
  apiBase.password ||
  apiBase.search ||
  apiBase.hash ||
  apiBase.pathname !== "/"
) {
  throw new Error("LV_API_BASE_URL 必须是无凭据的 HTTP(S) 服务根地址。");
}

const nextConfig: NextConfig = {
  // Self-contained server for the container image (OPS-01 §3).
  output: "standalone",
  poweredByHeader: false,
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${apiBase.origin}/api/:path*` },
    ];
  },
};

export default nextConfig;
