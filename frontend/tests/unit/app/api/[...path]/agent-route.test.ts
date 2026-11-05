import { afterEach, expect, it, vi } from "vitest";
import { GET, POST, PUT } from "@/app/api/[...path]/route";

const id = "00000000-0000-4000-8000-000000000001";
const base = `projects/${id}/agent`;
const session = `${base}/sessions/${id}`;
const run = `${session}/runs/${id}`;
const context = (path: string) => ({
  params: Promise.resolve({ path: path.split("/") }),
});
afterEach(() => vi.unstubAllGlobals());

it.each([
  ["GET", `${base}/sessions`, "limit=20&cursor=next"],
  ["POST", `${base}/sessions`, ""],
  ["GET", session, ""],
  ["GET", `${session}/runs`, "limit=20&cursor=next"],
  ["POST", `${session}/runs`, ""],
  ["GET", run, ""],
  ["POST", `${run}/commands`, ""],
  ["GET", `${run}/commands/current`, `request_id=${id}`],
  ["GET", `${run}/messages`, "limit=20&before=100&through=200"],
  ["GET", `${run}/messages/${id}`, ""],
  ["GET", `${session}/submissions/current`, ""],
  ["GET", `${session}/submissions/${id}`, ""],
  ["GET", `${base}/skills`, ""],
  ["POST", `${base}/skills`, ""],
  ["POST", `${base}/skills/${id}/versions/1.0.0/enabled`, ""],
])(
  "Agent %s %s reaches Go with its bounded contract",
  async (method, path, query) => {
    const fetcher = vi
      .fn()
      .mockImplementation(() =>
        Promise.resolve(Response.json({ accepted: true })),
      );
    vi.stubGlobal("fetch", fetcher);
    const request = new Request(
      `http://localhost/api/${path}${query ? `?${query}` : ""}`,
      {
        method,
        headers: { "X-Creation-Scope": id, "Content-Type": "application/json" },
        ...(method === "POST" ? { body: '{"test":"input"}' } : {}),
      },
    );
    const response = await (method === "GET" ? GET : POST)(
      request,
      context(path),
    );
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ accepted: true });
    const [url, init] = fetcher.mock.calls[0] as [URL, RequestInit];
    expect(url.pathname).toBe(`/api/${path}`);
    expect(url.searchParams.toString()).toBe(query);
    expect(new Headers(init.headers).get("X-Creation-Scope")).toBe(id);
    expect(init.method).toBe(method);
    if (method === "POST")
      expect(init.body?.toString()).toBe('{"test":"input"}');
  },
);

it("Agent proxy rejects ownership overrides, repeated queries and unsupported writes", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  for (const query of ["actor_id=other", "org_id=other", "limit=20&limit=50"])
    expect(
      (
        await GET(
          new Request(`http://localhost/api/${base}/sessions?${query}`),
          context(`${base}/sessions`),
        )
      ).status,
    ).toBe(422);
  expect(
    (
      await PUT(
        new Request(`http://localhost/api/${session}`, { method: "PUT" }),
        context(session),
      )
    ).status,
  ).toBe(405);
  expect(
    (
      await GET(
        new Request(`http://localhost/api/${base}/sessions/not-a-uuid`),
        context(`${base}/sessions/not-a-uuid`),
      )
    ).status,
  ).toBe(404);
  expect(fetcher).not.toHaveBeenCalled();
});

it("Agent events preserve resume cursor, streaming bytes and browser cancellation", async () => {
  const path = `${run}/events`;
  const abort = new AbortController();
  const fetcher = vi.fn().mockImplementation(() =>
    Promise.resolve(
      new Response("id: 42\ndata: {}\n\n", {
        headers: { "Content-Type": "text/event-stream" },
      }),
    ),
  );
  vi.stubGlobal("fetch", fetcher);
  const request = new Request(`http://localhost/api/${path}`, {
    headers: { "Last-Event-ID": "41" },
    signal: abort.signal,
  });
  const response = await GET(request, context(path));
  expect(response.status).toBe(200);
  expect(response.headers.get("Content-Type")).toBe("text/event-stream");
  expect(await response.text()).toBe("id: 42\ndata: {}\n\n");
  const init = fetcher.mock.calls[0][1] as RequestInit;
  expect(new Headers(init.headers).get("Last-Event-ID")).toBe("41");
  // Event subscriptions live until the browser cancels, beyond ordinary API deadlines.
  expect(init.signal).toBe(request.signal);
  abort.abort();
  expect(init.signal?.aborted).toBe(true);
});
