import { timingSafeEqual } from "node:crypto";

export function buildExpectedBearer(token: string | undefined): Buffer {
  if (typeof token !== "string" || Buffer.byteLength(token) < 32) {
    throw new Error("RUNTIME_TOKEN_REQUIRED");
  }
  return Buffer.from(`Bearer ${token}`);
}

export function verifyBearerToken(
  authorization: string | undefined,
  expected: Buffer,
): boolean {
  const actual = Buffer.from(authorization ?? "");
  return actual.length === expected.length && timingSafeEqual(actual, expected);
}
