import type { NextConfig } from "next";
import { getBackendOrigin } from "./src/server/backend-origin";

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  devIndicators: false,
  // Native launch capabilities are short-lived secrets; do not log their query.
  logging: {
    incomingRequests: {
      ignore: [/^(?:https?:\/\/[^/]+)?\/api\/mediaexports\/launch(?:\/|\?|$)/],
    },
  },
  allowedDevOrigins: ["127.0.0.1", "[::1]"],
  async redirects() {
    return [
      {
        source: "/subjects",
        destination: "/assets?view=subjects",
        permanent: false,
      },
    ];
  },
  async rewrites() {
    // Upgrade requests must reach Go before the fetch-only API catch-all.
    const backend = getBackendOrigin();
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
