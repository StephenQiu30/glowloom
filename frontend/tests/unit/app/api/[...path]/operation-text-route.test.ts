import { afterEach, expect, it, vi } from "vitest";
import { GET, POST } from "@/app/api/[...path]/route";

const id = "11111111-1111-4111-8111-111111111111";
const project = "22222222-2222-4222-8222-222222222222";
const path = `operations/${id}/text`;
const context = (value = path) => ({
  params: Promise.resolve({ path: value.split("/") }),
});
afterEach(() => vi.unstubAllGlobals());

it("forwards the complete original text with its session and project authorization", async () => {
  const text = "完整正文".repeat(20000);
  const fetcher = vi.fn().mockResolvedValue(
    new Response(text, {
      headers: {
        "Content-Type": "text/plain; charset=utf-8",
        "Content-Disposition": "attachment; filename=generated-text.txt",
        "X-Content-Type-Options": "nosniff",
      },
    }),
  );
  vi.stubGlobal("fetch", fetcher);
  const response = await GET(
    new Request(`http://localhost/api/${path}?project_id=${project}`, {
      headers: {
        Cookie: "session=synthetic",
        "X-Creation-Scope": id,
        "X-Actor-ID": "forged",
      },
    }),
    context(),
  );
  expect(response.status).toBe(200);
  expect(await response.text()).toBe(text);
  expect(response.headers.get("Content-Disposition")).toContain(
    "generated-text.txt",
  );
  expect(response.headers.get("Content-Type")).toContain("text/plain");
  expect(response.headers.get("Cache-Control")).toBe("no-store");
  expect(response.headers.get("X-Content-Type-Options")).toBe("nosniff");
  expect(fetcher).toHaveBeenCalledOnce();
  const [url, options] = fetcher.mock.calls[0];
  expect((url as URL).pathname).toBe(`/api/${path}`);
  expect((url as URL).searchParams.toString()).toBe(`project_id=${project}`);
  expect(options.method).toBe("GET");
  expect(options.headers.get("Cookie")).toBe("session=synthetic");
  expect(options.headers.get("X-Creation-Scope")).toBe(id);
  expect(options.headers.get("X-Actor-ID")).toBeNull();
});

it("keeps backend authorization failures and never invents downloadable text", async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValue(new Response('{"code":"forbidden"}', { status: 403 }));
  vi.stubGlobal("fetch", fetcher);
  const response = await GET(
    new Request(`http://localhost/api/${path}?project_id=${project}`),
    context(),
  );
  expect(response.status).toBe(403);
  expect(await response.json()).toEqual({ code: "forbidden" });
  expect(fetcher).toHaveBeenCalledOnce();
});

it("rejects extra query authority, duplicate project, writes and nested text routes locally", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  for (const query of [
    `project_id=${project}&project_id=${id}`,
    `project_id=${project}&actor_id=${id}`,
    `project_id=${project}&host=other`,
  ]) {
    expect(
      (
        await GET(
          new Request(`http://localhost/api/${path}?${query}`),
          context(),
        )
      ).status,
    ).toBe(422);
  }
  expect(
    (
      await POST(
        new Request(`http://localhost/api/${path}`, { method: "POST" }),
        context(),
      )
    ).status,
  ).toBe(405);
  expect(
    (
      await GET(
        new Request(`http://localhost/api/${path}/other`),
        context(`${path}/other`),
      )
    ).status,
  ).toBe(404);
  expect(fetcher).not.toHaveBeenCalled();
});
