import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  devIndicators: false,
  allowedDevOrigins: ["127.0.0.1"],
  async rewrites() {
    return [
      { source: "/favicon.ico", destination: "/brand/icons/favicon.ico" },
    ];
  },
};

export default nextConfig;
