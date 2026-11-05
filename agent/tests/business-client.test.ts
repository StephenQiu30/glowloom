import { afterEach, expect, it, vi } from "vitest";
import { BusinessClient } from "../src/business/client.js";
const client = new BusinessClient("http://fixture.invalid", "fixture");
afterEach(() => vi.unstubAllGlobals());
it("accepts a bounded image receipt and rejects a response beyond the shared 8 MiB limit", async () => {
  const image = "a".repeat(3 * 1024 * 1024);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ image }))),
  );
  expect(await client.request("/fixture")).toEqual({ image });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response('"' + "a".repeat(8 * 1024 * 1024) + '"')),
  );
  await expect(client.request("/fixture")).rejects.toMatchObject({
    code: "business_response_too_large",
  });
});
