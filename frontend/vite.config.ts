import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const apiTarget = env.VITE_API_URL || "http://localhost:8080";

  // Gateway secret sent to the backend in dev, mirroring the nginx proxy.
  // The tenant is NOT configured here — it's derived per-request from the host
  // the browser is on (see the proxyReq handler below), so one dev server can
  // serve any tenant by hostname, exactly like nginx's $host in production.
  const gatewayKey = env.VITE_GATEWAY_KEY || "";

  return {
    plugins: [react()],

    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },

    // ── /api proxy — eliminates CORS in development.
    // In production the Go server handles /api natively; no proxy needed.
    server: {
      port: 5173,
      proxy: {
        "/api": {
          target: apiTarget,
          changeOrigin: true,
          secure: false,
          configure: (proxy) => {
            proxy.on("proxyReq", (proxyReq, req) => {
              // Tenant = the domain the browser is actually on (mirrors nginx
              // `$host`). Derived per-request, never a fixed value — so one dev
              // server serves any tenant via its hostname (e.g. add a hosts
              // entry and visit acme.localhost:5173).
              const host = (req.headers.host || "").split(":")[0];
              if (host) proxyReq.setHeader("X-Tenant-Domain", host);
              if (gatewayKey) proxyReq.setHeader("X-Gateway-Key", gatewayKey);

              // SSE connections must not be buffered.
              if (req.headers.accept?.includes("text/event-stream")) {
                proxyReq.setHeader("Cache-Control", "no-cache");
                proxyReq.setHeader("X-Accel-Buffering", "no");
              }
            });
          },
        },
      },
    },

    build: {
      target: "es2020",
      sourcemap: false,
      rollupOptions: {
        output: {
          // Manual chunk splitting for optimal caching
          manualChunks: {
            "react-vendor": ["react", "react-dom"],
            "router": ["@tanstack/react-router"],
            "query": ["@tanstack/react-query"],
            "radix": [
              "@radix-ui/react-dialog",
              "@radix-ui/react-dropdown-menu",
              "@radix-ui/react-select",
              "@radix-ui/react-tabs",
              "@radix-ui/react-toast",
              "@radix-ui/react-tooltip",
            ],
            "motion": ["framer-motion"],
          },
        },
      },
      // Warn on chunks > 500kb
      chunkSizeWarningLimit: 500,
    },

    // Inline assets smaller than 4kb
    assetsInlineLimit: 4096,
  };
});
