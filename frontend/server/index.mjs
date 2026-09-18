// Production server for the CMS admin SPA (replaces nginx).
//
//   • Serves the built static assets from ../dist (with SPA history fallback).
//   • Reverse-proxies every /api/* request to the Go backend, stamping onto
//     each proxied call:
//       - X-Tenant-Domain: the domain the browser is on (per request → the
//         deployment stays multi-tenant), and
//       - X-Gateway-Key:   the shared secret so the backend can reject direct
//         or spoofed calls. Set here (server-side); never in the browser bundle.
//
// Zero runtime dependencies — Node stdlib only.
//
//   docker run -p 8080:3000 -e BACKEND_URL=http://app:8080 -e GATEWAY_API_KEY=… img

import { createServer, request as httpRequest } from "node:http";
import { request as httpsRequest } from "node:https";
import { createReadStream, statSync } from "node:fs";
import { join, normalize, extname, resolve, dirname, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { createGzip } from "node:zlib";

const __dirname = dirname(fileURLToPath(import.meta.url));
const DIST = resolve(__dirname, "..", "dist");

const PORT = Number(process.env.PORT || 3000);
const BACKEND_URL = process.env.BACKEND_URL || "http://app:8080";
const GATEWAY_API_KEY = process.env.GATEWAY_API_KEY || "";

const backend = new URL(BACKEND_URL);
const backendRequest = backend.protocol === "https:" ? httpsRequest : httpRequest;

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".map": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".gif": "image/gif",
  ".webp": "image/webp",
  ".ico": "image/x-icon",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
  ".ttf": "font/ttf",
  ".txt": "text/plain; charset=utf-8",
  ".webmanifest": "application/manifest+json",
};

const server = createServer((req, res) => {
  const url = req.url || "/";
  if (url === "/api" || url.startsWith("/api/")) {
    proxyApi(req, res, url);
  } else {
    serveStatic(req, res, url);
  }
});

// ── /api reverse proxy ──────────────────────────────────────────────────────
function proxyApi(req, res, url) {
  const host = (req.headers.host || "").split(":")[0];

  const headers = { ...req.headers };
  // Gateway contract — always overwrite so a client can't inject these.
  headers["x-tenant-domain"] = host;
  if (GATEWAY_API_KEY) headers["x-gateway-key"] = GATEWAY_API_KEY;
  else delete headers["x-gateway-key"];
  // Forwarding headers + point Host at the backend (like proxy changeOrigin).
  const ip = req.socket.remoteAddress || "";
  const xff = req.headers["x-forwarded-for"];
  headers["x-forwarded-for"] = xff ? `${xff}, ${ip}` : ip;
  headers["x-forwarded-proto"] = req.headers["x-forwarded-proto"] || "http";
  headers["x-real-ip"] = ip;
  headers.host = backend.host;

  const upstream = backendRequest(
    {
      protocol: backend.protocol,
      hostname: backend.hostname,
      port: backend.port || (backend.protocol === "https:" ? 443 : 80),
      method: req.method,
      path: url, // keeps /api/... and the query string intact
      headers,
    },
    (upRes) => {
      // Pipe straight through — no buffering, so SSE (/api/events) streams live.
      res.writeHead(upRes.statusCode || 502, upRes.headers);
      upRes.pipe(res);
    },
  );

  upstream.on("error", (err) => {
    if (!res.headersSent) {
      res.writeHead(502, { "content-type": "application/json; charset=utf-8" });
    }
    res.end(JSON.stringify({ error: "bad gateway", detail: err.message }));
  });

  req.pipe(upstream);
}

// ── Static files + SPA fallback ─────────────────────────────────────────────
function serveStatic(req, res, url) {
  let pathname;
  try {
    pathname = decodeURIComponent(new URL(url, "http://localhost").pathname);
  } catch {
    pathname = "/";
  }

  const requested = normalize(join(DIST, pathname));
  // Block path traversal outside DIST.
  if (requested !== DIST && !requested.startsWith(DIST + sep)) {
    res.writeHead(403, { "content-type": "text/plain" });
    res.end("Forbidden");
    return;
  }

  let isFile = false;
  try {
    isFile = statSync(requested).isFile();
  } catch {
    /* not found → fall through to SPA shell */
  }

  const filePath = isFile ? requested : join(DIST, "index.html");
  const cacheKey = isFile ? pathname : "/index.html";
  sendFile(req, res, filePath, cacheKey);
}

function sendFile(req, res, filePath, pathname) {
  let stat;
  try {
    stat = statSync(filePath);
  } catch {
    res.writeHead(404, { "content-type": "text/plain" });
    res.end("Not found");
    return;
  }

  const ext = extname(filePath).toLowerCase();
  const type = MIME[ext] || "application/octet-stream";
  const headers = { "content-type": type };

  if (pathname.startsWith("/assets/")) {
    headers["cache-control"] = "public, max-age=31536000, immutable";
  } else if (ext === ".html") {
    headers["cache-control"] = "no-cache";
  }

  const compressible = /text|javascript|json|svg|manifest/.test(type);
  const acceptsGzip = (req.headers["accept-encoding"] || "").includes("gzip");
  const stream = createReadStream(filePath);
  stream.on("error", () => {
    if (!res.headersSent) res.writeHead(500);
    res.end();
  });

  if (compressible && acceptsGzip) {
    headers["content-encoding"] = "gzip";
    headers["vary"] = "Accept-Encoding";
    res.writeHead(200, headers);
    stream.pipe(createGzip()).pipe(res);
  } else {
    headers["content-length"] = stat.size;
    res.writeHead(200, headers);
    stream.pipe(res);
  }
}

server.listen(PORT, () => {
  console.log(
    `[frontend] serving ${DIST} on :${PORT} — proxying /api → ${BACKEND_URL}` +
      (GATEWAY_API_KEY ? " (gateway key set)" : " (no gateway key — dev)"),
  );
});
