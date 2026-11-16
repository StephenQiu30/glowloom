import { afterEach, expect, it, vi } from "vitest";
import { GET, POST, PUT, PATCH } from "@/app/api/[...path]/route";
afterEach(() => vi.unstubAllGlobals());
const id = "11111111-1111-4111-8111-111111111111";
const context = (path: string) => ({
  params: Promise.resolve({ path: path.split("/") }),
});

it("通知读写经封闭BFF，保留过滤、水位和幂等键", async () => {
  const fetcher = vi
    .fn()
    .mockImplementation(() => Promise.resolve(new Response("{}")));
  vi.stubGlobal("fetch", fetcher);
  for (const path of [
    "announcements/public",
    "notifications",
    "notifications/summary",
    "notifications/display",
    `notifications/display/${id}/1`,
    `notifications/${id}`,
    "platform/notifications",
    `platform/notifications/${id}`,
    `platform/notifications/${id}/audit`,
  ]) {
    expect(
      (await GET(new Request(`http://localhost/api/${path}`), context(path)))
        .status,
    ).toBe(200);
  }
  for (const [method, handler, path] of [
    ["POST", POST, "notifications/read-all"],
    ["POST", POST, "notifications/display/ack"],
    ["POST", POST, `notifications/${id}/read`],
    ["POST", POST, "platform/notifications"],
    ["PUT", PUT, `platform/notifications/${id}`],
    ["POST", POST, `platform/notifications/${id}/publish`],
    ["PATCH", PATCH, `platform/notifications/${id}/enabled`],
  ] as const) {
    const body = JSON.stringify({
      command_id: id,
      data_epoch: id,
      through_seq: "9223372036854775",
    });
    expect(
      (
        await handler(
          new Request(`http://localhost/api/${path}`, {
            method,
            headers: {
              "Content-Type": "application/json",
              "Idempotency-Key": id,
              "X-CSRF-Token": "csrf",
            },
            body,
          }),
          context(path),
        )
      ).status,
    ).toBe(200);
    const [, init] = fetcher.mock.calls.at(-1)!;
    expect(init.headers.get("Idempotency-Key")).toBe(id);
    expect(init.headers.get("X-CSRF-Token")).toBe("csrf");
    expect(await new Response(init.body).text()).toBe(body);
  }
  expect(
    (
      await GET(
        new Request(
          "http://localhost/api/notifications?category=task&unread_only=true&limit=20",
        ),
        context("notifications"),
      )
    ).status,
  ).toBe(200);
  expect(
    (fetcher.mock.calls.at(-1)![0] as URL).searchParams.get("category"),
  ).toBe("task");
  fetcher.mockClear();
  expect(
    (
      await GET(
        new Request("http://localhost/api/notifications?user_id=foreign"),
        context("notifications"),
      )
    ).status,
  ).toBe(422);
  expect(
    (
      await GET(
        new Request(`http://localhost/api/notifications/${id}?owner=foreign`),
        context(`notifications/${id}`),
      )
    ).status,
  ).toBe(422);
  expect(fetcher).not.toHaveBeenCalled();
});
