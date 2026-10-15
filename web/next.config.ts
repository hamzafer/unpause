import type { NextConfig } from "next";

// Static export served by `unpause mcp stats --web` (ADR 0014).
// `pnpm build` writes web/out, then scripts/copy-export.mjs replaces
// ../internal/webui/dist with it. The build has to be reproducible, so the
// build id is fixed.
const mock = process.env.NEXT_PUBLIC_MOCK === "1";

const nextConfig: NextConfig = {
  output: "export",
  generateBuildId: async () => "unpause",
  images: { unoptimized: true },
  trailingSlash: false,
  poweredByHeader: false,
  productionBrowserSourceMaps: false,
  turbopack: {
    // Dev mock (fixtures) only with NEXT_PUBLIC_MOCK=1; a stub otherwise.
    resolveAlias: mock ? {} : { "@/lib/mock": "./lib/mock-off.ts" },
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
