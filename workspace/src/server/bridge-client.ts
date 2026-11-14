import { request } from "node:http";
import { getConfig } from "./config";
import { AccessError } from "./auth/session";
export function callBridge<T>(path: string, body: unknown): Promise<T> {
  const config = getConfig();
  if (!config.socket || !config.bridgeToken)
    throw new AccessError("bridge_unconfigured", 503);
  return new Promise((resolve, reject) => {
    const text = JSON.stringify(body);
    const req = request(
      {
        socketPath: config.socket,
        path,
        method: "POST",
        headers: {
          authorization: `Bearer ${config.bridgeToken}`,
          "content-type": "application/json",
          "content-length": Buffer.byteLength(text),
        },
        timeout: 30000,
      },
      (res) => {
        let result = "";
        res.setEncoding("utf8");
        res.on("data", (chunk: string) => {
          result += chunk;
          if (result.length > 3_000_000)
            req.destroy(new Error("Bridge response too large"));
        });
        res.on("end", () => {
          try {
            const data = JSON.parse(result);
            if (res.statusCode !== 200)
              reject(
                new AccessError(
                  data.error ?? "bridge_error",
                  res.statusCode ?? 503,
                ),
              );
            else resolve(data as T);
          } catch {
            reject(new AccessError("bridge_invalid_response", 502));
          }
        });
      },
    );
    req.on("error", () => reject(new AccessError("bridge_unknown", 503)));
    req.on("timeout", () => req.destroy(new Error("Bridge timeout")));
    req.end(text);
  });
}
