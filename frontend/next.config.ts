import type { NextConfig } from "next";
import { loadEnvConfig } from "@next/env";
import { resolve } from "node:path";

import { backendApiOrigin } from "./src/lib/backend-api-origin";

loadEnvConfig(resolve(process.cwd(), ".."));
const apiOrigin = backendApiOrigin(process.env.LV_API_BASE_URL);

const nextConfig: NextConfig = {
  // Self-contained server for the container image (OPS-01 §3).
  output: "standalone",
  poweredByHeader: false,
  async rewrites() {
    return {
      // The concrete streaming upload route must run before external rewrites.
      fallback: [
        { source: "/api/:path*", destination: `${apiOrigin}/api/:path*` },
      ],
    };
  },
};

export default nextConfig;
