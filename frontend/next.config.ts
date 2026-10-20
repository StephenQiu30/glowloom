import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  devIndicators: false,
  allowedDevOrigins: ["127.0.0.1"],
  async rewrites() {
    // Upgrade requests must reach Go before the fetch-only API catch-all.
    const backend = process.env.LV_BACKEND_ORIGIN ?? "http://127.0.0.1:8080";
    return {
      beforeFiles: [
        {
          source: "/api/canvases/:id/live",
          destination: `${backend}/api/canvases/:id/live`,
        },
      ],
      afterFiles: [
        { source: "/favicon.ico", destination: "/brand/icons/favicon.ico" },
      ],
      fallback: [],
    };
  },
};

export default nextConfig;
