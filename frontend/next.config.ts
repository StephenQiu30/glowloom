import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Self-contained server for the container image (OPS-01 §3).
  output: "standalone",
  poweredByHeader: false,
};

export default nextConfig;
