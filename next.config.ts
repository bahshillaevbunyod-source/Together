import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  images: {
    // Real posts carry avatar/media URLs served by the backend or an object
    // store. Allow remote hosts (localhost for dev, any https CDN in prod).
    remotePatterns: [
      { protocol: "http", hostname: "localhost" },
      { protocol: "https", hostname: "**" },
    ],
  },
};

export default nextConfig;
