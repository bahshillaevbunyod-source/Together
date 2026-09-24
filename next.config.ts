import type { NextConfig } from "next";

// Where the Next server reaches the Go backend for same-origin proxying. Used
// only when the browser is told to call the API on its own origin
// (NEXT_PUBLIC_API_BASE_URL=""), e.g. behind a single HTTPS host for
// real-device testing. Normal local dev keeps calling the backend directly.
const BACKEND_INTERNAL_URL = process.env.BACKEND_INTERNAL_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  images: {
    qualities: [85],
    // Real posts carry avatar/media URLs served by the backend or an object
    // store. Allow remote hosts (localhost for dev, any https CDN in prod).
    remotePatterns: [
      { protocol: "http", hostname: "localhost" },
      { protocol: "https", hostname: "**" },
    ],
  },
  async rewrites() {
    // Same-origin API + WebSocket proxy. The app has no Next routes under
    // /api, so this only forwards to the backend.
    return [{ source: "/api/:path*", destination: `${BACKEND_INTERNAL_URL}/api/:path*` }];
  },
};

export default nextConfig;
