// E2E harness server: serves the StartERP UI (compiled prototype or a
// rebuilt dist) and reverse-proxies the API so the UI talks to the adapter.
//   /v1/*  ->  ${API}/v1/erp/*   (the compiled prototype's prefix is fixed at /v1)
//   UI_DIR (dir with index.html) or UI_FILE (single html file)
// Usage: API=http://localhost:2010 UI_FILE=/path/index.html PORT=5180 node server.mjs
import http from "node:http";
import fs from "node:fs";
import path from "node:path";

const API = process.env.API || "http://localhost:2010";
const PORT = Number(process.env.PORT || 5180);
const UI_FILE = process.env.UI_FILE || "";
const UI_DIR = process.env.UI_DIR || "";
const PREFIX_FROM = process.env.PREFIX_FROM || "/v1/";
const PREFIX_TO = process.env.PREFIX_TO || "/v1/erp/";

const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png", ".json": "application/json", ".woff2": "font/woff2" };

function proxy(req, res) {
  const target = new URL(API);
  const p = req.url.startsWith(PREFIX_TO) ? req.url : PREFIX_TO + req.url.slice(PREFIX_FROM.length);
  const upstream = http.request({ host: target.hostname, port: target.port, method: req.method, path: p, headers: { ...req.headers, host: target.host } }, (r) => {
    res.writeHead(r.statusCode, r.headers);
    r.pipe(res);
  });
  upstream.on("error", (e) => { res.writeHead(502); res.end(String(e)); });
  req.pipe(upstream);
}

http.createServer((req, res) => {
  if (req.url.startsWith(PREFIX_FROM)) return proxy(req, res);
  let file = UI_FILE;
  if (UI_DIR) {
    const u = decodeURIComponent(req.url.split("?")[0]);
    const f = path.join(UI_DIR, path.normalize(u));
    file = fs.existsSync(f) && fs.statSync(f).isFile() ? f : path.join(UI_DIR, "index.html");
  }
  fs.readFile(file, (err, data) => {
    if (err) { res.writeHead(404); return res.end("not found"); }
    res.writeHead(200, { "Content-Type": types[path.extname(file)] || "application/octet-stream" });
    res.end(data);
  });
}).listen(PORT, () => console.log(`e2e server on :${PORT} -> ${API}${PREFIX_TO}`));
