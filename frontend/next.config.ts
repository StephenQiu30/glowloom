import { existsSync } from "node:fs";
import { resolve } from "node:path";

import type { NextConfig } from "next";

const repositoryEnvironmentFile = resolve(process.cwd(), "../.env");
if (existsSync(repositoryEnvironmentFile)) {
  process.loadEnvFile(repositoryEnvironmentFile);
}

const nextConfig: NextConfig = {
  allowedDevOrigins: ["127.0.0.1"],
  output: "standalone",
};

export default nextConfig;
