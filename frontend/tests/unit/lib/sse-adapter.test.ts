import { afterEach, expect, it, vi } from "vitest";
import { AxiosHeaders } from "axios";
import { sseAdapter } from "@/lib/request";
afterEach(() => vi.unstubAllGlobals());
it("uses the generated URL and current request scope, resumes the stored cursor, and parses complete SSE frames", async () => {
  const frames: unknown[] = [];
  const fetcher = vi.fn(
    async (_input: RequestInfo | URL, _options?: RequestInit) => {
      void _input;
      void _options;
      return new Response('id: 2:1\nevent: text\ndata: {"text":"hello"}\n\n', {
        headers: { "content-type": "text/event-stream" },
      });
    },
  );
  vi.stubGlobal("fetch", fetcher);
  const adapter = sseAdapter((frame) => frames.push(frame));
  await adapter({
    url: "/generated-current-path",
    method: "GET",
    headers: new AxiosHeaders({
      "X-Creation-Scope": "scope",
      "Last-Event-ID": "1:1",
    }),
  });
  expect(fetcher.mock.calls[0]?.[0]).toBe("/generated-current-path");
  expect(fetcher.mock.calls[0]?.[1]?.headers).toMatchObject({
    "X-Creation-Scope": "scope",
    "Last-Event-ID": "1:1",
  });
  expect(frames).toEqual([
    { id: "2:1", event: "text", data: { text: "hello" } },
  ]);
});
