import { createServer } from "node:http";
import { readFile, stat, realpath } from "node:fs/promises";
import { resolve, extname } from "node:path";
import { fileURLToPath } from "node:url";

const workspace = resolve(fileURLToPath(new URL("..", import.meta.url)));
const current = JSON.parse(
  await readFile(resolve(workspace, "dist/static/current.json"), "utf8"),
);
const root = await realpath(
  resolve(workspace, "dist/static", current.releaseId),
);
const release = JSON.parse(
  await readFile(resolve(root, "release.json"), "utf8"),
);
const types: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".txt": "text/plain; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".woff2": "font/woff2",
  ".wasm": "application/wasm",
  ".md": "text/plain; charset=utf-8",
  ".wav": "audio/wav",
  ".mp3": "audio/mpeg",
};
createServer(async (req, res) => {
  res.setHeader("X-Content-Type-Options", "nosniff");
  res.setHeader("Referrer-Policy", "same-origin");
  try {
    if (!["GET", "HEAD"].includes(req.method ?? "")) {
      res.writeHead(405).end();
      return;
    }
    let path = decodeURIComponent(
      new URL(req.url ?? "/", "http://localhost").pathname,
    );
    if (release.basePath) {
      if (
        !path.startsWith(release.basePath + "/") &&
        path !== release.basePath
      ) {
        res.writeHead(404).end();
        return;
      }
      path = path.slice(release.basePath.length) || "/";
    }
    let target = resolve(root, "." + path);
    if (!target.startsWith(root + "/") && target !== root)
      throw new Error("Path escape");
    try {
      if ((await stat(target)).isDirectory())
        target = resolve(target, "index.html");
    } catch {
      target = resolve(root, "404.html");
      res.statusCode = 404;
    }
    if (!(await realpath(target)).startsWith(root + "/"))
      throw new Error("Path escape");
    const bytes = await readFile(target);
    res.setHeader(
      "Content-Type",
      types[extname(target)] ?? "application/octet-stream",
    );
    const range = req.headers.range?.match(/^bytes=(\d+)-(\d*)$/);
    if (range) {
      const start = Number(range[1]),
        end = range[2] ? Number(range[2]) : bytes.length - 1;
      if (start > end || end >= bytes.length) {
        res.writeHead(416).end();
        return;
      }
      res.writeHead(206, {
        "Accept-Ranges": "bytes",
        "Content-Range": `bytes ${start}-${end}/${bytes.length}`,
        "Content-Length": end - start + 1,
      });
      res.end(
        req.method === "HEAD" ? undefined : bytes.subarray(start, end + 1),
      );
    } else {
      res.setHeader("Content-Length", bytes.length);
      res.end(req.method === "HEAD" ? undefined : bytes);
    }
  } catch {
    res.writeHead(404).end("Not found");
  }
}).listen(Number(process.env.WORKSPACE_PORT ?? 1131), "127.0.0.1", () =>
  console.log(
    `Static knowledge: http://127.0.0.1:${process.env.WORKSPACE_PORT ?? 1131}${release.basePath}/ (${current.releaseId})`,
  ),
);
