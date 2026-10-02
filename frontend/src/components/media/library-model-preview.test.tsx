import { cleanup, render, screen, waitFor, act } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { BoxGeometry, Mesh, MeshStandardMaterial, Scene, Texture } from "three";
import type { GLTF } from "three/addons/loaders/GLTFLoader.js";
import { LibraryModelPreview } from "./library-model-preview";
import { triangleJSON } from "@/components/canvas/director/model-test-fixtures";
const loader = vi.hoisted(() => ({ parse: vi.fn() }));
vi.mock("@/components/canvas/director/model-file", async (original) => ({
  ...(await original<
    typeof import("@/components/canvas/director/model-file")
  >()),
  parseDirectorModel: loader.parse,
}));
vi.mock("@react-three/fiber", () => ({
  Canvas: () => <div data-testid="model-ready" />,
}));
vi.mock("@react-three/drei", () => ({ OrbitControls: () => null }));
const bytes = triangleJSON();
function model() {
  const geometry = new BoxGeometry(),
    material = new MeshStandardMaterial();
  const texture = new Texture();
  material.map = texture;
  const scene = new Scene();
  scene.add(new Mesh(geometry, material));
  return {
    gltf: { scene, animations: [] } as unknown as GLTF,
    geometry,
    material,
    texture,
  };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() => Promise.resolve(new Response(bytes))),
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
it("JSON原件完整读完后按canonicalMIME解析，生命周期释放几何/材质/纹理", async () => {
  const owned = model();
  loader.parse.mockResolvedValue(owned.gltf);
  const geometry = vi.spyOn(owned.geometry, "dispose"),
    material = vi.spyOn(owned.material, "dispose"),
    texture = vi.spyOn(owned.texture, "dispose");
  const error = vi.fn();
  const view = render(
    <LibraryModelPreview
      url="https://storage.example/current-signed"
      byteSize={bytes.byteLength}
      mimeType="model/gltf+json"
      onError={error}
    />,
  );
  await screen.findByTestId("model-ready");
  expect(loader.parse).toHaveBeenCalledWith(
    expect.any(ArrayBuffer),
    "model/gltf+json",
  );
  expect(new Uint8Array(loader.parse.mock.calls[0][0])).toEqual(
    new Uint8Array(bytes),
  );
  expect(fetch).toHaveBeenCalledWith(
    "https://storage.example/current-signed",
    expect.objectContaining({
      credentials: "omit",
      redirect: "error",
      signal: expect.any(AbortSignal),
    }),
  );
  expect(error).not.toHaveBeenCalled();
  view.unmount();
  expect(geometry).toHaveBeenCalledOnce();
  expect(material).toHaveBeenCalledOnce();
  expect(texture).toHaveBeenCalledOnce();
});
it("关闭后才解析完成的模型立即释放，不更新UI或报告错误", async () => {
  const owned = model(),
    geometry = vi.spyOn(owned.geometry, "dispose");
  let release!: (value: GLTF) => void;
  loader.parse.mockImplementation(
    () =>
      new Promise((resolve) => {
        release = resolve;
      }),
  );
  const error = vi.fn();
  const view = render(
    <LibraryModelPreview
      url="https://storage.example/current-signed"
      byteSize={bytes.byteLength}
      mimeType="model/gltf+json"
      onError={error}
    />,
  );
  await waitFor(() => expect(loader.parse).toHaveBeenCalledOnce());
  const signal = vi.mocked(fetch).mock.calls[0][1]?.signal;
  view.unmount();
  expect(signal?.aborted).toBe(true);
  await act(async () => release(owned.gltf));
  expect(geometry).toHaveBeenCalledOnce();
  expect(error).not.toHaveBeenCalled();
});
it("截断或解析失败报告当前错误，不挂半个模型", async () => {
  const error = vi.fn();
  render(
    <LibraryModelPreview
      url="https://storage.example/current-signed"
      byteSize={bytes.byteLength + 1}
      mimeType="model/gltf+json"
      onError={error}
    />,
  );
  await waitFor(() => expect(error).toHaveBeenCalledOnce());
  expect(loader.parse).not.toHaveBeenCalled();
  expect(screen.queryByTestId("model-ready")).toBeNull();
});
it("本地完整JSON File沿同一个解析器，不请求URL也不制造转换Blob", async () => {
  loader.parse.mockResolvedValue(model().gltf);
  const error = vi.fn(),
    original = new File([bytes], "triangle.gltf", { type: "model/gltf+json" });
  render(
    <LibraryModelPreview
      file={original}
      byteSize={original.size}
      mimeType="model/gltf+json"
      onError={error}
    />,
  );
  await screen.findByTestId("model-ready");
  expect(fetch).not.toHaveBeenCalled();
  expect(new Uint8Array(loader.parse.mock.calls[0][0])).toEqual(
    new Uint8Array(bytes),
  );
  expect(error).not.toHaveBeenCalled();
});
