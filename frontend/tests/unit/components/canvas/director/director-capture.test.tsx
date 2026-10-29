import {
  activeDirectorScene,
  migrateDirectorDocument,
} from "@/components/canvas/director/director-workspaces";
import { createElement, type ImgHTMLAttributes } from "react";
import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import {
  DirectorCapture,
  DirectorGallery,
  bindDirectorCapture,
} from "@/components/canvas/director/director-capture";
import {
  emptyDirectorConfig,
  type Director,
} from "@/components/canvas/director/director-model";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
const api = vi.hoisted(() => ({
  downloadMediaOriginal: vi.fn(),
  uploadMediaAsset: vi.fn(),
  getMediaPreview: vi.fn(),
}));
vi.mock("@/api/media", () => api);
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
}));
// jsdom does not decode Next images; pixel decoding is covered by browser acceptance.
vi.mock("next/image", () => ({
  default: ({
    unoptimized: _unoptimized,
    ...props
  }: ImgHTMLAttributes<HTMLImageElement> & { unoptimized?: boolean }) => {
    void _unoptimized;
    return createElement("img", props);
  },
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
  vi.unstubAllGlobals();
});
const projectID = "10000000-0000-4000-8000-000000000001",
  nodeID = "10000000-0000-4000-8000-000000000002",
  assetID = "10000000-0000-4000-8000-000000000003";
const asset = (): API.AssetSummary => ({
  id: assetID,
  project_id: projectID,
  kind: "image",
  width: 1920,
  height: 1080,
});
const output = (): Required<Omit<API.DirectorScreenshot, "kind">> => ({
  id: crypto.randomUUID(),
  asset_id: assetID,
  name: "机位拍摄",
  created_at: new Date().toISOString(),
});
function mount(withHistory = false) {
  const ExistingURL = URL;
  vi.stubGlobal(
    "URL",
    class extends ExistingURL {
      static createObjectURL = vi.fn(() => "blob:http://localhost/capture");
      static revokeObjectURL = vi.fn();
    },
  );
  const baseline = emptyDirectorConfig();
  let current = baseline,
    epoch = "epoch-1";
  const client = {
    getDocumentEpoch: () => epoch,
    retainedDirector: () => null,
    getSnapshot: () => ({
      writable: true,
      document: {
        nodes: [{ id: nodeID, serviceData: { config: { director: current } } }],
      },
    }),
    waitForSaved: vi.fn(async (check?: () => void) => {
      check?.();
      return { revision: "2" };
    }),
    changeDirector: vi.fn((_id: string, next: Director) => {
      current = next;
    }),
  };
  const capture = vi
      .fn()
      .mockResolvedValue(new Blob(["mocked-png-bytes"], { type: "image/png" })),
    onError = vi.fn();
  const onCommit = vi.fn((next: API.DirectorConfig) => {
    current = next as Director;
    return true;
  });
  render(
    <DirectorCapture
      client={client as unknown as CanvasYjsLiveSession}
      nodeID={nodeID}
      projectID={projectID}
      scene={baseline}
      capture={capture}
      disabled={false}
      onPause={vi.fn()}
      onError={onError}
      onCommit={withHistory ? onCommit : undefined}
    />,
  );
  return {
    client,
    capture,
    onError,
    onCommit,
    getCurrent: () => current,
    setEpoch: (value: string) => {
      epoch = value;
    },
    setCurrent: (value: Director) => {
      current = value;
    },
  };
}
async function shootAndReview() {
  fireEvent.click(screen.getByRole("button", { name: "拍摄" }));
  const image = await screen.findByRole("img", { name: "实际机位拍摄图片" });
  Object.defineProperty(image, "naturalWidth", {
    configurable: true,
    value: 1920,
  });
  Object.defineProperty(image, "naturalHeight", {
    configurable: true,
    value: 1080,
  });
  fireEvent.load(image);
  fireEvent.click(screen.getByRole("checkbox"));
}
it("binds only the exact authorized image and rejects duplicate, malformed or missing shot output", () => {
  const scene = emptyDirectorConfig(),
    shot = scene.active_shot_id,
    image = output();
  const next = bindDirectorCapture(scene, shot, image, asset(), projectID);
  expect(next.shots[0].screenshots).toEqual([image]);
  expect(scene.shots[0].screenshots).toBeUndefined();
  expect(() =>
    bindDirectorCapture(next, shot, image, asset(), projectID),
  ).toThrow("已存在");
  expect(() =>
    bindDirectorCapture(scene, crypto.randomUUID(), image, asset(), projectID),
  ).toThrow("不存在");
  for (const wrong of [
    { ...asset(), project_id: crypto.randomUUID() },
    { ...asset(), kind: "video" },
    { ...asset(), id: crypto.randomUUID() },
  ])
    expect(() =>
      bindDirectorCapture(scene, shot, image, wrong, projectID),
    ).toThrow("回执不匹配");
  expect(() =>
    bindDirectorCapture(
      scene,
      shot,
      { ...image, created_at: "yesterday" },
      asset(),
      projectID,
    ),
  ).toThrow("身份无效");
});
it("waits for accepted scene, requires image review, uploads once and saves durable shot identity", async () => {
  api.uploadMediaAsset.mockResolvedValue({ asset: asset() });
  const f = mount();
  fireEvent.click(screen.getByRole("button", { name: "拍摄" }));
  const image = await screen.findByRole("img", { name: "实际机位拍摄图片" });
  expect(f.client.waitForSaved).toHaveBeenCalledTimes(1);
  expect(
    (
      screen.getByRole("button", {
        name: "保存到镜头素材",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  expect(api.uploadMediaAsset).not.toHaveBeenCalled();
  Object.defineProperty(image, "naturalWidth", {
    configurable: true,
    value: 1920,
  });
  Object.defineProperty(image, "naturalHeight", {
    configurable: true,
    value: 1080,
  });
  fireEvent.load(image);
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "保存到镜头素材" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(api.uploadMediaAsset.mock.calls[0][0]).toEqual({ pid: projectID });
  expect(api.uploadMediaAsset.mock.calls[0][1]).toEqual({
    local_review_confirmed: true,
  });
  expect(f.client.changeDirector).toHaveBeenCalledTimes(1);
  expect(
    activeDirectorScene(migrateDirectorDocument(f.getCurrent())).shots[0]
      .screenshots?.[0].asset_id,
  ).toBe(assetID);
  expect(f.client.waitForSaved).toHaveBeenCalledTimes(2);
});
it("reuses the original upload File and idempotency key after unknown network outcome", async () => {
  api.uploadMediaAsset
    .mockRejectedValueOnce(new Error("upload unknown"))
    .mockResolvedValueOnce({ asset: asset() });
  mount();
  await shootAndReview();
  fireEvent.click(screen.getByRole("button", { name: "保存到镜头素材" }));
  await screen.findByText("upload unknown");
  fireEvent.click(screen.getByRole("button", { name: "保存到镜头素材" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(api.uploadMediaAsset).toHaveBeenCalledTimes(2);
  expect(api.uploadMediaAsset.mock.calls[0][2]).toBe(
    api.uploadMediaAsset.mock.calls[1][2],
  );
  expect(api.uploadMediaAsset.mock.calls[0][3].headers).toEqual(
    api.uploadMediaAsset.mock.calls[1][3].headers,
  );
});
it.each(["epoch", "scene"])(
  "does not publish an old capture after %s changes",
  async (kind) => {
    const f = mount();
    await shootAndReview();
    if (kind === "epoch") f.setEpoch("epoch-2");
    else f.setCurrent({ ...f.getCurrent(), background: "#ffffff" });
    fireEvent.click(screen.getByRole("button", { name: "保存到镜头素材" }));
    await screen.findByText("场景或画布版本已变化，请重新拍摄");
    expect(api.uploadMediaAsset).not.toHaveBeenCalled();
    expect(f.client.changeDirector).not.toHaveBeenCalled();
  },
);
it("rejects a foreign upload result without binding it to the scene", async () => {
  api.uploadMediaAsset.mockResolvedValue({
    asset: { ...asset(), project_id: crypto.randomUUID() },
  });
  const f = mount();
  await shootAndReview();
  fireEvent.click(screen.getByRole("button", { name: "保存到镜头素材" }));
  await screen.findByText("图片上传回执不匹配");
  expect(f.client.changeDirector).not.toHaveBeenCalled();
});

it("routes output through workspace history and preserves upload receipt on rejected commit", async () => {
  api.uploadMediaAsset.mockResolvedValue({ asset: asset() });
  const f = mount(true);
  f.onCommit.mockReturnValueOnce(false);
  await shootAndReview();
  fireEvent.click(screen.getByRole("button", { name: "保存到镜头素材" }));
  await screen.findByText("镜头素材未写入场景，请处理恢复稿");
  expect(f.client.changeDirector).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "保存到镜头素材" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(api.uploadMediaAsset).toHaveBeenCalledTimes(1);
  expect(f.onCommit).toHaveBeenCalledTimes(2);
  expect(f.onCommit.mock.calls[1][0]).toEqual(f.onCommit.mock.calls[0][0]);
  expect(f.client.changeDirector).not.toHaveBeenCalled();
});

it.each([
  ["镜头 2.webm", "video/mp4", "镜头 2.mp4"],
  ["镜头 2.WEBM", "video/webm; codecs=vp9", "镜头 2.webm"],
  ["机位图片", "image/png", "机位图片.png"],
  ["分镜.版本2.webm", "video/mp4", "分镜.版本2.mp4"],
  ["镜头 2.webm", "application/octet-stream", "镜头 2.webm"],
])(
  "downloads %s using actual %s without renaming the saved shot",
  async (name, mime, expected) => {
    const ExistingURL = URL;
    vi.stubGlobal(
      "URL",
      class extends ExistingURL {
        static createObjectURL = vi.fn(() => "blob:http://localhost/output");
        static revokeObjectURL = vi.fn();
      },
    );
    const scene = emptyDirectorConfig();
    scene.shots[0].screenshots = [{ ...output(), name, kind: "video" }];
    const onChange = vi.fn();
    api.downloadMediaOriginal.mockResolvedValue(
      new Blob(["decoded media"], { type: mime }),
    );
    const names: string[] = [];
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(function (this: HTMLAnchorElement) {
        names.push(this.download);
      });
    try {
      render(
        <DirectorGallery
          scene={scene}
          projectID={projectID}
          disabled={false}
          onChange={onChange}
        />,
      );
      fireEvent.click(screen.getByRole("button", { name: "镜头素材" }));
      fireEvent.click(screen.getByRole("checkbox", { name: `选择 ${name}` }));
      fireEvent.click(screen.getByRole("button", { name: "下载" }));
      await waitFor(() => expect(names).toEqual([expected]));
      expect(scene.shots[0].screenshots[0].name).toBe(name);
      expect(onChange).not.toHaveBeenCalled();
      expect(api.downloadMediaOriginal).toHaveBeenCalledWith(
        { pid: projectID, asset_id: assetID },
        expect.objectContaining({ responseType: "blob" }),
      );
    } finally {
      click.mockRestore();
    }
  },
);
