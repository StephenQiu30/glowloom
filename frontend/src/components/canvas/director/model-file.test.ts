import { afterEach, expect, it, vi } from "vitest";
import {
  BoxGeometry,
  Mesh,
  MeshStandardMaterial,
  Points,
  Scene,
  Texture,
} from "three";
import type { GLTF } from "three/addons/loaders/GLTFLoader.js";
import {
  parseDirectorModel,
  readModelOriginal,
  disposeDirectorModel,
} from "./model-file";
import {
  triangleDocument,
  triangleGLB,
  triangleJSON,
} from "./model-test-fixtures";

afterEach(() => vi.restoreAllMocks());
it.each([
  ["model/gltf+json", triangleJSON],
  ["model/gltf-binary", triangleGLB],
])(
  "真实GLTFLoader从完整%s原件解析几何，不转换容器或读取外部URL",
  async (mime, original) => {
    const fetchOriginal = globalThis.fetch;
    const requests = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input, options) => {
        const url =
          typeof input === "string"
            ? input
            : input instanceof URL
              ? input.href
              : input.url;
        if (!url.startsWith("data:")) throw new Error("禁止测试访问外部资源");
        return fetchOriginal(input, options);
      });
    const bytes = original();
    const before = new Uint8Array(bytes).slice();
    const loaded = await parseDirectorModel(bytes, mime);
    const mesh = loaded.scene.getObjectByName("合成三角模型😀");
    expect(mesh).toBeInstanceOf(Mesh);
    const geometry = (mesh as Mesh).geometry;
    expect(geometry.getAttribute("position").count).toBe(3);
    expect(Array.from(geometry.getAttribute("position").array)).toEqual([
      0, 0, 0, 1, 0, 0, 0, 1, 0,
    ]);
    expect(new Uint8Array(bytes)).toEqual(before);
    expect(
      requests.mock.calls.every(([input]) =>
        String(input instanceof Request ? input.url : input).startsWith(
          "data:",
        ),
      ),
    ).toBe(true);
    geometry.dispose();
  },
);
it.each([
  "https://example.invalid/private.bin",
  "http://127.0.0.1/extra",
  "../geometry.bin",
  "geometry.bin",
  "file:///private.bin",
  "blob:unowned",
])("包外URI%s在启动Loader前拒绝，0请求", async (uri) => {
  const document = triangleDocument();
  document.buffers[0].uri = uri;
  const request = vi.spyOn(globalThis, "fetch");
  await expect(
    parseDirectorModel(triangleJSON(document), "model/gltf+json"),
  ).rejects.toThrow();
  expect(request).not.toHaveBeenCalled();
});
it("原件按实际字节完整读取，截断/多字节/cancel均取消reader且无半成品", async () => {
  const bytes = new Uint8Array(triangleJSON());
  const stream = () =>
    new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(bytes.slice(0, 5));
        controller.enqueue(bytes.slice(5));
        controller.close();
      },
    });
  const signal = new AbortController().signal;
  expect(
    new Uint8Array(await readModelOriginal(stream(), bytes.length, signal)),
  ).toEqual(bytes);
  await expect(
    readModelOriginal(stream(), bytes.length + 1, signal),
  ).rejects.toThrow("完整");
  await expect(
    readModelOriginal(stream(), bytes.length - 1, signal),
  ).rejects.toThrow("容量");
  const cancelled = new AbortController();
  cancelled.abort();
  await expect(
    readModelOriginal(stream(), bytes.length, cancelled.signal),
  ).rejects.toMatchObject({ name: "AbortError" });
});
it("容量异常拒绝读取，stream错误仍释放reader", async () => {
  const signal = new AbortController().signal;
  for (const expected of [0, -1, 1.5, 64 * 1024 * 1024 + 1])
    await expect(
      readModelOriginal(new Response("x").body!, expected, signal),
    ).rejects.toThrow();
  const failure = new Error("原件传输中断");
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.error(failure);
    },
  });
  await expect(readModelOriginal(stream, 1, signal)).rejects.toThrow(failure);
  expect(stream.locked).toBe(false);
});
it("等待下一块时abort取消reader，释放锁并拒绝发布部分原件", async () => {
  const cancellation = vi.fn();
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new Uint8Array([1]));
    },
    cancel: cancellation,
  });
  const controller = new AbortController();
  const result = readModelOriginal(stream, 2, controller.signal);
  await Promise.resolve();
  controller.abort();
  await expect(result).rejects.toMatchObject({ name: "AbortError" });
  expect(cancellation).toHaveBeenCalledOnce();
  expect(stream.locked).toBe(false);
});
it("每块超额立即取消reader并释放锁，不继续读取或拼接", async () => {
  const cancellation = vi.fn();
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new Uint8Array([1, 2, 3]));
    },
    cancel: cancellation,
  });
  await expect(
    readModelOriginal(stream, 2, new AbortController().signal),
  ).rejects.toThrow("容量");
  expect(cancellation).toHaveBeenCalledOnce();
  expect(stream.locked).toBe(false);
});
it("所有场景的共享几何、材质与纹理只释放一次，Points也纳入模型生命周期", () => {
  const geometry = new BoxGeometry(),
    material = new MeshStandardMaterial(),
    texture = new Texture();
  material.map = texture;
  const first = new Scene(),
    second = new Scene();
  first.add(new Mesh(geometry, material));
  second.add(new Points(geometry, material));
  const disposeGeometry = vi.spyOn(geometry, "dispose"),
    disposeMaterial = vi.spyOn(material, "dispose"),
    disposeTexture = vi.spyOn(texture, "dispose");
  disposeDirectorModel({
    scene: first,
    scenes: [first, second],
  } as unknown as GLTF);
  expect(disposeGeometry).toHaveBeenCalledOnce();
  expect(disposeMaterial).toHaveBeenCalledOnce();
  expect(disposeTexture).toHaveBeenCalledOnce();
});
