import { createServer } from "node:http";
import { timingSafeEqual } from "node:crypto";
import { pathToFileURL } from "node:url";
import { computeCandidate } from "./service-runtime.mjs";

export function runtimeServer({ token, maxConcurrent = 4 }) {
  if (typeof token !== "string" || Buffer.byteLength(token) < 32)
    throw new Error("RUNTIME_TOKEN_REQUIRED");
  const expected = Buffer.from(`Bearer ${token}`);
  let active = 0;
  return createServer(
    { requestTimeout: 15000, headersTimeout: 5000, maxHeaderSize: 8192 },
    async (req, res) => {
      const send = (status, value) => {
        res.writeHead(status, {
          "Content-Type": "application/json",
          "Cache-Control": "no-store",
        });
        res.end(JSON.stringify(value));
      };
      if (req.method === "GET" && req.url === "/healthz") {
        send(200, { status: "ok" });
        return;
      }
      const actual = Buffer.from(req.headers.authorization ?? "");
      if (
        actual.length !== expected.length ||
        !timingSafeEqual(actual, expected)
      ) {
        send(401, { code: "UNAUTHORIZED" });
        req.resume();
        return;
      }
      if (
        req.method !== "POST" ||
        !["/v1/initialize", "/v1/prepare"].includes(req.url)
      ) {
        send(404, { code: "NOT_FOUND" });
        req.resume();
        return;
      }
      if (active >= maxConcurrent) {
        send(503, { code: "BUSY", definitive: false });
        req.resume();
        return;
      }
      active++;
      try {
        const chunks = [];
        let size = 0;
        for await (const chunk of req) {
          size += chunk.length;
          if (size > 16 * 1024 * 1024) {
            send(413, { code: "REQUEST_TOO_LARGE", definitive: false });
            return;
          }
          chunks.push(chunk);
        }
        let request;
        try {
          request = JSON.parse(Buffer.concat(chunks).toString("utf8"));
        } catch {
          send(400, { code: "INVALID_JSON", definitive: false });
          return;
        }
        const candidate = await computeCandidate(req.url.slice(4), request);
        send(200, candidate);
      } catch (error) {
        send(error.definitive ? 422 : 503, {
          code:
            typeof error.code === "string" ? error.code : "RUNTIME_UNAVAILABLE",
          definitive: error.definitive === true,
        });
      } finally {
        active--;
      }
    },
  );
}
if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const server = runtimeServer({
    token: process.env.GLOWLOOM_COLLAB_RUNTIME_TOKEN,
  });
  server.listen(
    Number(process.env.PORT ?? 4400),
    process.env.HOST ?? "127.0.0.1",
  );
  for (const signal of ["SIGTERM", "SIGINT"])
    process.once(signal, () => server.close());
}
