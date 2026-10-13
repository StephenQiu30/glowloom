import { afterEach, expect, it, vi } from "vitest";
import { uploadPersonalMediaAsset } from "./generated/media";
afterEach(() => vi.unstubAllGlobals());
it("生成上传客户端保持FormData与确认字段，不将文件序列化为JSON", async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValue(new Response('{"asset":{}}', { status: 201 }));
  vi.stubGlobal("fetch", fetcher);
  const file = new File(["data"], "flower.png", { type: "image/png" });
  await uploadPersonalMediaAsset({ local_review_confirmed: true }, file, {
    headers: { "Idempotency-Key": "same-upload" },
  });
  const [url, init] = fetcher.mock.calls[0];
  expect(url).toBe("/api/media/library/uploads");
  expect(init.body).toBeInstanceOf(FormData);
  expect(init.body.get("file").name).toBe("flower.png");
  expect(init.body.get("local_review_confirmed")).toBe("true");
  expect(init.headers.has("Content-Type")).toBe(false);
  expect(init.headers.get("Idempotency-Key")).toBe("same-upload");
});
