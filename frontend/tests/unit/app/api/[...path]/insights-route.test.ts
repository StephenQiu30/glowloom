import { afterEach, expect, it, vi } from "vitest";
import { GET, POST } from "@/app/api/[...path]/route";
const context = (path: string) => ({
  params: Promise.resolve({ path: path.split("/") }),
});
afterEach(() => vi.unstubAllGlobals());
it("forwards only bounded diagnostic and analytic reads with no-store and actor cookie", async () => {
  const fetcher = vi.fn(async () => new Response("{}"));
  vi.stubGlobal("fetch", fetcher);
  for (const [path, query] of [
    ["platform/diagnostics/overview", "?service=backend&level=error"],
    [
      "platform/diagnostics/logs",
      "?request_id=11111111-1111-4111-8111-111111111111&cursor=signed",
    ],
    ["platform/diagnostics/logs/11111111-1111-4111-8111-111111111111", ""],
    ["analytics/creation", "?scope=personal&origin=canvas"],
    ["analytics/business", ""],
  ] as const) {
    const response = await GET(
      new Request(`http://localhost/api/${path}${query}`, {
        headers: { Cookie: "fixture-session" },
      }),
      context(path),
    );
    expect(response.status).toBe(200);
    const [url, init] = fetcher.mock.calls.at(-1)! as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.search).toBe(query);
    expect(new Headers(init.headers).get("Cookie")).toBe("fixture-session");
    expect(init.cache).toBe("no-store");
  }
});
it("rejects unknown, duplicate, write and internal ingest paths without reaching upstream", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  for (const path of [
    "platform/diagnostics/ingest",
    "internal/diagnostics/logs",
    "analytics/export",
  ])
    expect(
      (await GET(new Request(`http://localhost/api/${path}`), context(path)))
        .status,
    ).toBe(404);
  for (const [path, query] of [
    ["platform/diagnostics/overview", "?service=backend&service=backend"],
    ["platform/diagnostics/logs", "?environment=production"],
    ["analytics/creation", "?role=platform_admin"],
    ["analytics/business", "?scope=platform"],
    [
      "platform/diagnostics/logs/11111111-1111-4111-8111-111111111111",
      "?id=other",
    ],
  ])
    expect(
      (
        await GET(
          new Request(`http://localhost/api/${path}${query}`),
          context(path),
        )
      ).status,
    ).toBe(422);
  expect(
    (
      await POST(
        new Request("http://localhost/api/analytics/creation", {
          method: "POST",
        }),
        context("analytics/creation"),
      )
    ).status,
  ).toBe(405);
  expect(fetcher).not.toHaveBeenCalled();
});
