import { afterEach, expect, it, vi } from "vitest";
import { GET, POST } from "@/app/api/[...path]/route";

afterEach(() => vi.unstubAllGlobals());
const path = "canvas-locations/00000000-0000-4000-8000-000000000001";
const context = (resource: string) => ({
  params: Promise.resolve({ path: resource.split("/") }),
});

it("forwards canvas location reads with session and no cache", async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValue(Response.json({ scope_id: "scope" }));
  vi.stubGlobal("fetch", fetcher);
  const response = await GET(
    new Request(`http://localhost/api/${path}`, {
      headers: { cookie: "glowloom_session=test" },
    }),
    context(path),
  );
  expect(response.status).toBe(200);
  expect((fetcher.mock.calls[0][0] as URL).pathname).toBe(`/api/${path}`);
  expect(new Headers(fetcher.mock.calls[0][1].headers).get("cookie")).toContain(
    "glowloom_session=test",
  );
  expect(response.headers.get("cache-control")).toBe("no-store");
});

it("rejects writes, malformed IDs and routing query overrides before forwarding", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  expect(
    (
      await POST(
        new Request(`http://localhost/api/${path}`, { method: "POST" }),
        context(path),
      )
    ).status,
  ).toBe(405);
  expect(
    (
      await GET(
        new Request("http://localhost/api/canvas-locations/wrong"),
        context("canvas-locations/wrong"),
      )
    ).status,
  ).toBe(404);
  expect(
    (
      await GET(
        new Request(`http://localhost/api/${path}?scope_id=other`),
        context(path),
      )
    ).status,
  ).toBe(422);
  expect(fetcher).not.toHaveBeenCalled();
});
