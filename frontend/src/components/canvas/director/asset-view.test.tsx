import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { BoxGeometry, Mesh, MeshStandardMaterial, Scene } from "three";
import type { GLTF } from "three/addons/loaders/GLTFLoader.js";
import { AuthorizedModel } from "./asset-view";
import { triangleJSON } from "./model-test-fixtures";
const owner = vi.hoisted(() => ({ preview: vi.fn(), parse: vi.fn() }));
vi.mock("../queries", () => ({ getMediaPreview: owner.preview }));
vi.mock("./model-file", async (original) => ({
  ...(await original<typeof import("./model-file")>()),
  parseDirectorModel: owner.parse,
}));
vi.mock("@react-three/fiber", () => ({ useFrame: vi.fn() }));
vi.mock("@react-three/drei", () => ({
  Html: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));
const bytes = triangleJSON(),
  project = "11111111-1111-4111-8111-111111111111",
  id = "22222222-2222-4222-8222-222222222222";
const lease = {
  asset: {
    id,
    project_id: project,
    kind: "model",
    mime_type: "model/gltf+json",
    byte_size: bytes.byteLength,
  },
  url: "https://storage.example/fresh-signed-original",
  expires_at: new Date(Date.now() + 600000).toISOString(),
};
function model() {
  const geometry = new BoxGeometry(),
    material = new MeshStandardMaterial(),
    scene = new Scene();
  scene.add(new Mesh(geometry, material));
  return {
    gltf: { scene, animations: [] } as unknown as GLTF,
    geometry,
    material,
  };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() => Promise.resolve(new Response(bytes))),
  );
  owner.preview.mockResolvedValue(lease);
  owner.parse.mockResolvedValue(model().gltf);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function mount(
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  mode: "beauty" | "clay" = "beauty",
) {
  const loaded = vi.fn(),
    status = vi.fn();
  return {
    ...render(
      <QueryClientProvider client={client}>
        <AuthorizedModel
          projectId={project}
          assetId={id}
          mode={mode}
          onLoaded={loaded}
          onStatus={status}
        />
      </QueryClientProvider>,
    ),
    loaded,
    status,
  };
}
it("warm签名URL不能绕过fresh项目授权；新事实完成后才按真实MIME与整件字节读取", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["canvas", "director-asset", project, id, "model"], {
    ...lease,
    url: "https://storage.example/old-signed",
  });
  let release!: (value: typeof lease) => void;
  owner.preview.mockImplementation(
    () =>
      new Promise((resolve) => {
        release = resolve;
      }),
  );
  const view = mount(client);
  expect(fetch).not.toHaveBeenCalled();
  expect(owner.parse).not.toHaveBeenCalled();
  await act(async () => release(lease));
  await waitFor(() => expect(view.loaded).toHaveBeenCalledOnce());
  expect(fetch).toHaveBeenCalledWith(
    lease.url,
    expect.objectContaining({ credentials: "omit", redirect: "error" }),
  );
  expect(owner.parse).toHaveBeenCalledWith(
    expect.any(ArrayBuffer),
    "model/gltf+json",
  );
  expect(new Uint8Array(owner.parse.mock.calls[0][0])).toEqual(
    new Uint8Array(bytes),
  );
  expect(view.status).toHaveBeenLastCalledWith(true);
});
it.each([
  { asset: { ...lease.asset, project_id: id } },
  { asset: { ...lease.asset, id: project } },
  { url: "file:///private.gltf" },
  { expires_at: "2000-01-01T00:00:00Z" },
])("身份或租约异常拒绝模型原件请求 %j", async (change) => {
  owner.preview.mockResolvedValue({ ...lease, ...change });
  const view = mount();
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(fetch).not.toHaveBeenCalled();
  expect(view.loaded).not.toHaveBeenCalled();
});
it("正常关闭释放完整模型；截断原件不能进入ready", async () => {
  const own = model(),
    geometry = vi.spyOn(own.geometry, "dispose"),
    material = vi.spyOn(own.material, "dispose");
  owner.parse.mockResolvedValue(own.gltf);
  const view = mount();
  await waitFor(() => expect(view.loaded).toHaveBeenCalledOnce());
  view.unmount();
  expect(geometry).toHaveBeenCalledOnce();
  expect(material).toHaveBeenCalledOnce();
  expect(view.status).toHaveBeenLastCalledWith(false);
  owner.parse.mockClear();
  vi.mocked(fetch).mockResolvedValue(
    new Response(new Uint8Array(bytes).slice(0, -1)),
  );
  const broken = mount();
  expect((await screen.findByRole("alert")).textContent).toContain("完整");
  expect(owner.parse).not.toHaveBeenCalled();
  expect(broken.loaded).not.toHaveBeenCalled();
});
it("材质预览关闭先恢复原材质，再释放原件所有资源", async () => {
  const own = model(),
    geometry = vi.spyOn(own.geometry, "dispose"),
    material = vi.spyOn(own.material, "dispose");
  owner.parse.mockResolvedValue(own.gltf);
  const view = mount(undefined, "clay");
  await waitFor(() => expect(view.loaded).toHaveBeenCalledOnce());
  const mesh = own.gltf.scene.children[0] as Mesh,
    replacement = vi.spyOn(mesh.material as MeshStandardMaterial, "dispose");
  expect(mesh.material).not.toBe(own.material);
  view.unmount();
  expect(material).toHaveBeenCalledOnce();
  expect(geometry).toHaveBeenCalledOnce();
  expect(replacement).toHaveBeenCalledOnce();
});
