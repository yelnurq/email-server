import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  distDir: process.env.NEXT_DIST_DIR || ".next",
  // Pin the Turbopack root to this app. Without it Turbopack walks up looking
  // for a lockfile and can settle on a directory far above the repo (any stray
  // package-lock.json in a parent), which pulls unrelated sibling projects into
  // module resolution and filesystem watching and makes cold compiles crawl.
  turbopack: {
    root: __dirname,
  },
  async rewrites() {
    const api = process.env.API_INTERNAL_URL || "http://localhost:8080";
    return [{ source: "/backend/:path*", destination: `${api}/:path*` }];
  },
};

export default nextConfig;
