import { afterEach, expect, it, vi } from "vitest";
import { GET } from "@/app/api/mediaexports/launch/route";
import config from "../../../../next.config";
const token = "a".repeat(64);
const url = `http://localhost/api/mediaexports/launch?token=${token}`;
afterEach(() => vi.unstubAllGlobals());
it("relays only the exact native capability, without browser credentials or upstream cookies", async () => {
  const body = {
    short_video_draft: {
      name: "时间线",
      draft_url: "https://media.example/signed.zip",
    },
  };
  const fetcher = vi
    .fn()
    .mockResolvedValue(
      Response.json(body, { headers: { "Set-Cookie": "synthetic=ignored" } }),
    );
  vi.stubGlobal("fetch", fetcher);
  const response = await GET(
    new Request(url, {
      headers: {
        Cookie: "session=synthetic",
        Authorization: "Bearer synthetic",
        "X-CSRF-Token": "synthetic",
      },
    }),
  );
  expect(response.status).toBe(200);
  expect(await response.json()).toEqual(body);
  const [upstream, options] = fetcher.mock.calls[0] as [URL, RequestInit];
  expect(upstream.pathname).toBe("/api/media-export-launch");
  expect([...upstream.searchParams]).toEqual([["token", token]]);
  expect(new Headers(options.headers).get("Cookie")).toBeNull();
  expect(new Headers(options.headers).get("Authorization")).toBeNull();
  expect(options).toMatchObject({
    credentials: "omit",
    cache: "no-store",
    redirect: "manual",
  });
  expect(response.headers.get("Set-Cookie")).toBeNull();
  expect(response.headers.get("Cache-Control")).toBe("no-store");
  expect(response.headers.get("Referrer-Policy")).toBe("no-referrer");
});
it("rejects malformed, duplicate and extra capability selectors before network access", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  for (const query of [
    "",
    "token=bad",
    `token=${"A".repeat(64)}`,
    `token=${token}&token=${token}`,
    `token=${token}&job_id=x`,
    `token=${token}&url=https://other.example`,
  ]) {
    expect(
      (
        await GET(
          new Request(`http://localhost/api/mediaexports/launch?${query}`),
        )
      ).status,
    ).toBe(422);
  }
  expect((await GET(new Request(url, { method: "POST" }))).status).toBe(405);
  expect(fetcher).not.toHaveBeenCalled();
});
it("never redirects a native capability and bounds unexpected upstream responses", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  for (const response of [
    new Response(null, {
      status: 302,
      headers: { Location: "https://other.example" },
    }),
    new Response("html", { headers: { "Content-Type": "text/html" } }),
    new Response("{}", {
      headers: { "Content-Type": "application/json-malformed" },
    }),
    Response.json("wrong"),
    new Response(" ".repeat(16385), {
      headers: { "Content-Type": "application/json" },
    }),
  ]) {
    fetcher.mockResolvedValueOnce(response);
    expect((await GET(new Request(url))).status).toBe(502);
  }
  fetcher.mockResolvedValueOnce(new Response(null, { status: 410 }));
  expect((await GET(new Request(url))).status).toBe(410);
});
it("suppresses only native capability request logging, including token queries", () => {
  const patterns = (
    config.logging as { incomingRequests: { ignore: RegExp[] } }
  ).incomingRequests.ignore;
  expect(
    patterns.some((p) => p.test(`/api/mediaexports/launch?token=${token}`)),
  ).toBe(true);
  expect(patterns.some((p) => p.test("/api/media-exports/job/preview"))).toBe(
    false,
  );
});
