export type Capability = "read" | "edit" | "run" | "accept" | "publish";
export type Principal = {
  actor: string;
  capabilities: Capability[];
  expiresAt: number;
  nonce: string;
};
export const cookieName = "glowloom_workspace_session";
export class AccessError extends Error {
  constructor(
    public code: string,
    public status: number,
  ) {
    super(code);
  }
}
const mac = (value: string, secret: string) =>
  createHmac("sha256", secret).update(value).digest("base64url");
const equal = (a: string, b: string) => {
  const left = Buffer.from(a),
    right = Buffer.from(b);
  return left.length === right.length && timingSafeEqual(left, right);
};
export function issueSession(
  secret: string,
  caps: Capability[],
  now = Date.now(),
): string {
  if (secret.length < 32) throw new AccessError("session_unconfigured", 503);
  const body = Buffer.from(
    JSON.stringify({
      actor: "local-owner",
      capabilities: caps,
      expiresAt: now + 30 * 60 * 1000,
      nonce: randomBytes(18).toString("base64url"),
    }),
  ).toString("base64url");
  return body + "." + mac(body, secret);
}
export function verifySession(
  token: string,
  secret: string,
  now = Date.now(),
): Principal | null {
  if (secret.length < 32 || token.length > 4096) return null;
  const [body, signature, extra] = token.split(".");
  if (!body || !signature || extra || !equal(signature, mac(body, secret)))
    return null;
  try {
    const p = JSON.parse(
      Buffer.from(body, "base64url").toString(),
    ) as Principal;
    if (
      p.actor !== "local-owner" ||
      !Array.isArray(p.capabilities) ||
      !p.nonce ||
      p.expiresAt <= now ||
      p.expiresAt > now + 30 * 60 * 1000
    )
      return null;
    return p;
  } catch {
    return null;
  }
}
export function csrfFor(token: string, secret: string): string {
  return mac("csrf:" + token, secret);
}
export function authorize(
  request: {
    token: string;
    origin: string;
    expectedOrigin: string;
    csrf: string;
  },
  secret: string,
  cap: Capability,
  now = Date.now(),
  write = false,
): Principal {
  const principal = verifySession(request.token, secret, now);
  if (!principal) throw new AccessError("unauthorized", 401);
  if (!principal.capabilities.includes(cap))
    throw new AccessError("forbidden", 403);
  if (write) {
    if (!request.expectedOrigin || request.origin !== request.expectedOrigin)
      throw new AccessError("origin_mismatch", 403);
    if (!equal(request.csrf, csrfFor(request.token, secret)))
      throw new AccessError("csrf_mismatch", 403);
  }
  return principal;
}
import { createHmac, randomBytes, timingSafeEqual } from "node:crypto";
