import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  BoxGeometry,
  Mesh,
  MeshStandardMaterial,
  Scene,
  Texture,
  type Material,
} from "three";
import type { GLTF } from "three/addons/loaders/GLTFLoader.js";
import { AuthorizedModel } from "./asset-view";
import { triangleJSON } from "./model-test-fixtures";

const owner = vi.hoisted(() => ({
  preview: vi.fn(),
  parse: vi.fn(),
  resource: vi.fn(),
}));
vi.mock("@/lib/request", () => ({ readResourceStream: owner.resource }));
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
  projectId = "11111111-1111-4111-8111-111111111111",
  assetId = "22222222-2222-4222-8222-222222222222",
  key = ["canvas", "director-asset", projectId, assetId, "model"],
  lease = {
    asset: {
      id: assetId,
      project_id: projectId,
      kind: "model",
      mime_type: "model/gltf+json",
      byte_size: bytes.byteLength,
    },
    url: "https://storage.example/initial-original",
    expires_at: new Date(Date.now() + 600000).toISOString(),
  };

function ownedModel() {
  const geometry = new BoxGeometry(),
    texture = new Texture(),
    material = new MeshStandardMaterial({ map: texture }),
    mesh = new Mesh(geometry, material),
    scene = new Scene();
  scene.add(mesh);
  return {
    gltf: { scene, animations: [] } as unknown as GLTF,
    mesh,
    material,
    geometryDispose: vi.spyOn(geometry, "dispose"),
    materialDispose: vi.spyOn(material, "dispose"),
    textureDispose: vi.spyOn(texture, "dispose"),
  };
}

beforeEach(() => {
  vi.resetAllMocks();
  owner.resource.mockImplementation(() =>
    Promise.resolve(new Response(bytes).body!),
  );
  owner.preview.mockResolvedValue(lease);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

async function mount(mode: "clay" | "normal" | "depth" | "pose" = "clay") {
  const model = ownedModel(),
    client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
    loaded = vi.fn(),
    status = vi.fn();
  owner.parse.mockResolvedValue(model.gltf);
  const element = (currentMode = mode) => (
    <QueryClientProvider client={client}>
      <AuthorizedModel
        projectId={projectId}
        assetId={assetId}
        mode={currentMode}
        onLoaded={loaded}
        onStatus={status}
      />
    </QueryClientProvider>
  );
  const view = render(element());
  // onLoaded runs before the committed material Effect. Observe its actual
  // replacement before testing cleanup, rather than relying on callback timing.
  await waitFor(() => expect(model.mesh.material).not.toBe(model.material));
  return {
    ...view,
    model,
    client,
    loaded,
    status,
    element,
    replacementDispose: vi.spyOn(model.mesh.material as Material, "dispose"),
  };
}

function expectDisposedOnce(model: ReturnType<typeof ownedModel>) {
  expect.soft(model.geometryDispose).toHaveBeenCalledOnce();
  expect.soft(model.materialDispose).toHaveBeenCalledOnce();
  expect.soft(model.textureDispose).toHaveBeenCalledOnce();
}

it.each(["clay", "normal", "depth", "pose"] as const)(
  "%s 模式续租开始即恢复材质并完整释放原件，拒绝续租不保留旧对象",
  async (mode) => {
    const view = await mount(mode);
    let reject!: (reason: Error) => void;
    owner.preview.mockImplementation(
      () =>
        new Promise((_resolve, fail) => {
          reject = fail;
        }),
    );
    let refetch!: Promise<unknown>;
    await act(async () => {
      refetch = view.client.refetchQueries({ queryKey: key });
    });
    await waitFor(() =>
      expect(view.model.geometryDispose).toHaveBeenCalledOnce(),
    );
    expectDisposedOnce(view.model);
    expect(view.replacementDispose).toHaveBeenCalledOnce();
    expect(view.status).toHaveBeenLastCalledWith(false);
    expect(view.loaded).toHaveBeenCalledOnce();
    expect(owner.resource).toHaveBeenCalledOnce();
    await act(async () => {
      reject(new Error("项目预览授权已撤回"));
      await refetch;
    });
    expect((await screen.findByRole("alert")).textContent).toContain("撤回");
    view.rerender(view.element("normal"));
    expectDisposedOnce(view.model);
    expect(view.replacementDispose).toHaveBeenCalledOnce();
    view.unmount();
    expectDisposedOnce(view.model);
    expect(view.replacementDispose).toHaveBeenCalledOnce();
  },
);

it("签名 URL 直接轮换后新加载失败，旧模型原始 texture/material 仍只释放一次", async () => {
  const view = await mount();
  owner.resource.mockRejectedValueOnce(new Error("新原件读取失败"));
  await act(async () => {
    view.client.setQueryData(key, {
      ...lease,
      url: "https://storage.example/rotated-original",
    });
  });
  expect((await screen.findByRole("alert")).textContent).toContain("新原件");
  expectDisposedOnce(view.model);
  expect(view.replacementDispose).toHaveBeenCalledOnce();
  expect(view.loaded).toHaveBeenCalledOnce();
  view.unmount();
  expectDisposedOnce(view.model);
  expect(view.replacementDispose).toHaveBeenCalledOnce();
});

it("签名 URL 轮换成功时旧资源完整释放，新材质切换与关闭仍各自单次释放", async () => {
  const view = await mount(),
    fresh = ownedModel();
  owner.parse.mockResolvedValue(fresh.gltf);
  await act(async () => {
    view.client.setQueryData(key, {
      ...lease,
      url: "https://storage.example/rotated-original",
    });
  });
  await waitFor(() => expect(view.loaded).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(fresh.mesh.material).not.toBe(fresh.material));
  expectDisposedOnce(view.model);
  expect(view.replacementDispose).toHaveBeenCalledOnce();
  const clayDispose = vi.spyOn(fresh.mesh.material as Material, "dispose");
  view.rerender(view.element("normal"));
  expect(clayDispose).toHaveBeenCalledOnce();
  expect(fresh.materialDispose).not.toHaveBeenCalled();
  expect(fresh.textureDispose).not.toHaveBeenCalled();
  const normalDispose = vi.spyOn(fresh.mesh.material as Material, "dispose");
  view.unmount();
  expectDisposedOnce(fresh);
  expect(clayDispose).toHaveBeenCalledOnce();
  expect(normalDispose).toHaveBeenCalledOnce();
});

it("同 URL 和时间戳的新租约对象也完整释放旧模型，失败期间切换模式不重新分配旧材质", async () => {
  const view = await mount(),
    updatedAt = view.client.getQueryState(key)!.dataUpdatedAt;
  owner.resource.mockRejectedValueOnce(new Error("新原件读取失败"));
  await act(async () => {
    view.client.setQueryData(
      key,
      { ...lease, expires_at: new Date(Date.now() + 700000).toISOString() },
      { updatedAt },
    );
  });
  expect((await screen.findByRole("alert")).textContent).toContain("新原件");
  expectDisposedOnce(view.model);
  view.rerender(view.element("normal"));
  expect(view.model.mesh.material).toBe(view.model.material);
  view.unmount();
  expectDisposedOnce(view.model);
  expect(view.replacementDispose).toHaveBeenCalledOnce();
});

it("关闭后迟到的新模型只释放解析资源，不发布 ready 或重获材质", async () => {
  const view = await mount(),
    late = ownedModel();
  let resolve!: (value: GLTF) => void;
  owner.parse.mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  await act(async () => {
    view.client.setQueryData(key, {
      ...lease,
      url: "https://storage.example/late-original",
    });
  });
  await waitFor(() => expect(owner.parse).toHaveBeenCalledTimes(2));
  view.unmount();
  await act(async () => resolve(late.gltf));
  expectDisposedOnce(late);
  expect(view.loaded).toHaveBeenCalledOnce();
  expect(view.status).toHaveBeenLastCalledWith(false);
});
