import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import { CanvasDirectorWorkspace } from "@/components/canvas/director/canvas-director-workspace";
import { emptyDirectorConfig } from "@/components/canvas/director/director-model";
import {
  activeDirectorScene,
  migrateDirectorDocument,
  switchDirectorWorkspace,
} from "@/components/canvas/director/director-workspaces";
import type { DirectorImportDialogProps } from "@/components/canvas/director/director-import-dialog";
import type { DirectorViewport } from "@/components/canvas/director/director-viewport";
import type { ProjectAssetDialog } from "@/components/media/project-asset-dialog";

const captured = vi.hoisted(() => ({
  viewport: null as ComponentProps<typeof DirectorViewport> | null,
  importer: null as DirectorImportDialogProps | null,
  picker: null as ComponentProps<typeof ProjectAssetDialog> | null,
  preview: vi.fn(),
}));
vi.mock("@/api/media", () => ({ getMediaPreview: captured.preview }));
vi.mock("@/api/canvases", () => ({
  readDirectorRecovery: vi.fn().mockResolvedValue({ draft: null }),
  saveDirectorRecovery: vi.fn().mockResolvedValue({ revision: 1 }),
  deleteDirectorRecovery: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("@/components/canvas/director/director-project-assets", () => ({
  DirectorProjectAssets: () => null,
}));
vi.mock("next/dynamic", () => ({
  default: () => (props: ComponentProps<typeof ProjectAssetDialog>) => {
    captured.picker = props;
    return <div data-testid="asset-picker" />;
  },
}));
vi.mock("@/components/canvas/director/director-viewport", () => ({
  DirectorViewport: (props: ComponentProps<typeof DirectorViewport>) => {
    captured.viewport = props;
    return null;
  },
}));
vi.mock("@/components/canvas/director/director-import-dialog", () => ({
  DirectorImportDialog: (props: DirectorImportDialogProps) => {
    captured.importer = props;
    return <div data-testid="local-import" />;
  },
}));
vi.mock("@/components/canvas/director/director-capture", () => ({
  DirectorCapture: () => null,
  DirectorGallery: () => null,
}));
vi.mock("@/components/canvas/director/director-view-preferences", () => ({
  directorHelpSeen: () => true,
  markDirectorHelpSeen: () => {},
}));

const projectID = "10000000-0000-4000-8000-000000000001";
const model = {
  id: "10000000-0000-4000-8000-000000000002",
  project_id: projectID,
  kind: "model",
  file_name: "模型.glb",
} as API.AssetSummary;
const imageAssets = ["背景甲", "背景乙"].map(
  (name, index) =>
    ({
      ...model,
      id: `10000000-0000-4000-8000-00000000000${index + 3}`,
      kind: "image",
      file_name: name,
    }) as API.AssetSummary,
);
function mount() {
  let config: API.DirectorConfig = migrateDirectorDocument(
    emptyDirectorConfig(),
  );
  let epoch = "opening-epoch";
  let writable = true;
  const listeners = new Set<() => void>();
  const snapshot = () => ({
    writable,
    migration: false,
    document: {
      nodes: [
        { id: "director", serviceData: { config: { director: config } } },
        ...imageAssets.map((asset) => ({
          id: asset.id!,
          type: "image",
          title: asset.file_name,
          serviceData: { media_ref: { asset_id: asset.id } },
        })),
      ],
    },
  });
  let view = snapshot();
  const notify = () => {
    view = snapshot();
    listeners.forEach((listener) => listener());
  };
  const client = {
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    getSnapshot: () => view,
    getDocumentEpoch: () => epoch,
    retainedDirector: () => null,
    retainDirector: vi.fn(),
    discardRetainedDirector: vi.fn(),
    waitForSaved: vi.fn(async () => ({
      nodes: [
        { id: "director", config: { director: structuredClone(config) } },
      ],
    })),
    changeDirector: vi.fn(
      (
        _id: string,
        next: API.DirectorConfig,
        baseline: API.DirectorConfig,
        expectedEpoch: string,
      ) => {
        if (!writable || epoch !== expectedEpoch || baseline !== config)
          throw new Error("场景版本冲突");
        config = next;
        notify();
      },
    ),
  };
  const rendered = render(
    <CanvasDirectorWorkspace
      client={
        client as unknown as ComponentProps<
          typeof CanvasDirectorWorkspace
        >["client"]
      }
      nodeID="director"
      canvasID="canvas"
      projectID={projectID}
    />,
  );
  return {
    unmount: rendered.unmount,
    client,
    scene: () => activeDirectorScene(migrateDirectorDocument(config)),
    changeScope: (scope: "epoch" | "mode" | "readonly" | "conflict") =>
      act(() => {
        if (scope === "epoch") epoch = "new-epoch";
        if (scope === "mode")
          config = switchDirectorWorkspace(
            migrateDirectorDocument(config),
            "dynamic",
            false,
            0,
          );
        if (scope === "readonly") writable = false;
        if (scope === "conflict") {
          const concurrent = migrateDirectorDocument(config);
          concurrent.workspaces.static.background = "#ffffff";
          config = concurrent;
        }
        notify();
      }),
  };
}
const undo = () =>
  screen.getByRole("button", {
    name: "撤销",
  }) as HTMLButtonElement;
async function importModel() {
  fireEvent.click(screen.getByRole("button", { name: "导入本地角色" }));
  await act(async () => {
    captured.importer!.guard();
    await captured.importer!.onAdd(model);
    captured.importer!.onClose();
  });
}
beforeEach(() => {
  captured.viewport = null;
  captured.importer = null;
  captured.picker = null;
  captured.preview.mockReset();
});
afterEach(cleanup);

it("stages an uploaded model, places it once at the ground coordinate and records undo/redo", async () => {
  const { client, scene } = mount();
  await importModel();
  expect(client.changeDirector).not.toHaveBeenCalled();
  expect(client.waitForSaved).not.toHaveBeenCalled();
  expect(undo().disabled).toBe(true);
  expect(captured.viewport!.placement).toBeTruthy();
  const place = captured.viewport!.onPlace!;
  act(() => {
    place([3, 0.5, 7]);
    place([3, 0.5, 7]);
  });
  expect(client.changeDirector).toHaveBeenCalledOnce();
  expect(scene().objects[0]).toMatchObject({
    kind: "actor",
    asset_id: model.id,
    transform: { position: [3, 0.5, 7] },
  });
  expect(undo().disabled).toBe(false);
  fireEvent.click(undo());
  expect(scene().objects).toEqual([]);
  fireEvent.click(screen.getByRole("button", { name: "重做" }));
  expect(scene().objects).toHaveLength(1);
});

it("cancels uploaded-model placement without inserting or removing its asset", async () => {
  const { client, scene } = mount();
  await importModel();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(captured.viewport!.placement).toBeUndefined();
  expect(scene().objects).toEqual([]);
  expect(client.changeDirector).not.toHaveBeenCalled();
});

it.each(["epoch", "mode", "readonly"] as const)(
  "blocks uploaded-model handoff after %s changes",
  async (scope) => {
    const { client, changeScope } = mount();
    fireEvent.click(screen.getByRole("button", { name: "导入本地角色" }));
    changeScope(scope);
    await expect(captured.importer!.onAdd(model)).rejects.toThrow();
    expect(client.changeDirector).not.toHaveBeenCalled();
    expect(captured.viewport!.placement).toBeUndefined();
  },
);

it.each(["epoch", "mode", "readonly", "conflict"] as const)(
  "does not insert a staged model after %s changes",
  async (scope) => {
    const { scene, changeScope } = mount();
    await importModel();
    changeScope(scope);
    act(() => captured.viewport!.onPlace!([1, 2, 3]));
    expect(scene().objects).toEqual([]);
    expect(screen.getByRole("alert").textContent).toBeTruthy();
  },
);

it("queues project models as actors and preserves remaining assets when placement is cancelled", async () => {
  const { client, scene } = mount();
  fireEvent.click(screen.getByRole("button", { name: "从项目选择角色" }));
  await act(async () =>
    captured.picker!.onAdd([
      model,
      { ...model, id: "10000000-0000-4000-8000-000000000003" },
    ]),
  );
  expect(screen.queryByTestId("asset-picker")).toBeNull();
  expect(client.changeDirector).not.toHaveBeenCalled();
  act(() => captured.viewport!.onPlace!([1, 0, 2]));
  expect(scene().objects).toHaveLength(1);
  expect(scene().objects[0].kind).toBe("actor");
  expect(captured.viewport!.placement).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(scene().objects).toHaveLength(1);
  fireEvent.click(undo());
  expect(scene().objects).toEqual([]);
});

it("places a project multi-selection in separate undoable transactions", async () => {
  const { client, scene } = mount();
  fireEvent.click(screen.getByRole("button", { name: "从项目选择角色" }));
  await act(async () =>
    captured.picker!.onAdd([
      model,
      { ...model, id: "10000000-0000-4000-8000-000000000003" },
    ]),
  );
  act(() => captured.viewport!.onPlace!([1, 0, 2]));
  act(() => captured.viewport!.onPlace!([3, 0, 4]));
  expect(client.changeDirector).toHaveBeenCalledTimes(2);
  expect(scene().objects.map((object) => object.transform.position)).toEqual([
    [1, 0, 2],
    [3, 0, 4],
  ]);
  expect(captured.viewport!.placement).toBeUndefined();
  fireEvent.click(undo());
  expect(scene().objects).toHaveLength(1);
  fireEvent.click(undo());
  expect(scene().objects).toEqual([]);
});

it("places a flat background as a billboard, with no panorama mutation", async () => {
  const { client, scene } = mount();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "背景" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(screen.getByRole("button", { name: "添加背景" }));
  fireEvent.click(screen.getByRole("button", { name: "从项目选择" }));
  await act(async () => captured.picker!.onAdd([{ ...model, kind: "image" }]));
  expect(client.changeDirector).not.toHaveBeenCalled();
  act(() => captured.viewport!.onPlace!([2, 1, 3]));
  expect(scene().objects[0]).toMatchObject({
    kind: "billboard",
    asset_id: model.id,
    transform: { position: [2, 1, 3] },
  });
  expect(scene().panorama).toBeUndefined();
});

it("rejects a mixed-project batch before staging any object", async () => {
  const { client } = mount();
  fireEvent.click(screen.getByRole("button", { name: "从项目选择角色" }));
  await expect(
    captured.picker!.onAdd([model, { ...model, project_id: "other-project" }]),
  ).rejects.toThrow("素材归属");
  expect(client.changeDirector).not.toHaveBeenCalled();
  expect(captured.viewport!.placement).toBeUndefined();
});

it("uses the palette name for a newly placed cube", () => {
  const { scene } = mount();
  fireEvent.mouseDown(screen.getAllByRole("tab", { name: "物体" })[0], {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(screen.getByRole("button", { name: /立方体/ }));
  act(() => captured.viewport!.onPlace!([0, 0.5, 0]));
  expect(scene().objects[0].name).toBe("立方体");
});

it.each(["epoch", "mode", "readonly"] as const)(
  "blocks project-asset confirmation after %s changes",
  async (scope) => {
    const { client, changeScope } = mount();
    fireEvent.click(screen.getByRole("button", { name: "从项目选择角色" }));
    changeScope(scope);
    await expect(captured.picker!.onAdd([model])).rejects.toThrow();
    expect(client.changeDirector).not.toHaveBeenCalled();
  },
);

it("saves a panorama directly with undo, without creating a ground-placement object", async () => {
  const { client, scene } = mount();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "背景" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(screen.getByRole("button", { name: "添加背景" }));
  fireEvent.mouseDown(
    within(screen.getByRole("dialog")).getByRole("tab", { name: "全景背景" }),
    { button: 0, ctrlKey: false },
  );
  fireEvent.click(screen.getByRole("button", { name: "从项目选择" }));
  await act(async () => captured.picker!.onAdd([{ ...model, kind: "image" }]));
  expect(client.changeDirector).toHaveBeenCalledOnce();
  expect(scene().panorama?.asset_id).toBe(model.id);
  expect(captured.viewport!.placement).toBeUndefined();
  fireEvent.click(undo());
  expect(scene().panorama).toBeUndefined();
});

function openCanvasBackground(kind: "billboard" | "panorama" = "panorama") {
  fireEvent.mouseDown(screen.getByRole("tab", { name: "背景" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(screen.getByRole("button", { name: "添加背景" }));
  fireEvent.mouseDown(
    within(screen.getByRole("dialog")).getByRole("tab", {
      name: kind === "panorama" ? "全景背景" : "平面背景",
    }),
    { button: 0, ctrlKey: false },
  );
  fireEvent.click(screen.getByRole("button", { name: "从画布选择" }));
}
function pendingPreview() {
  let resolve!: (value: API.Preview) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<API.Preview>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  captured.preview.mockReturnValueOnce(promise);
  return { resolve, reject };
}
function previewSignal(index = 0): AbortSignal | undefined {
  return captured.preview.mock.calls[index]?.[1]?.signal;
}
it.each(["billboard", "panorama"] as const)(
  "cancels a pending canvas %s after closing without applying its late response",
  async (kind) => {
    const { client, scene } = mount();
    const pending = pendingPreview();
    openCanvasBackground(kind);
    fireEvent.click(screen.getByRole("button", { name: "背景甲" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    await act(async () => pending.resolve({ asset: imageAssets[0] }));
    expect(client.changeDirector).not.toHaveBeenCalled();
    expect(scene().panorama).toBeUndefined();
    expect(captured.viewport!.placement).toBeUndefined();
    expect(previewSignal()?.aborted).toBe(true);
  },
);
it("only applies the latest canvas background selection when responses arrive out of order", async () => {
  const { client, scene } = mount();
  const first = pendingPreview();
  const second = pendingPreview();
  openCanvasBackground();
  fireEvent.click(screen.getByRole("button", { name: "背景甲" }));
  fireEvent.click(screen.getByRole("button", { name: "背景乙" }));
  await act(async () => second.resolve({ asset: imageAssets[1] }));
  await act(async () => first.resolve({ asset: imageAssets[0] }));
  expect(client.changeDirector).toHaveBeenCalledOnce();
  expect(scene().panorama?.asset_id).toBe(imageAssets[1].id);
  expect(previewSignal()?.aborted).toBe(true);
  fireEvent.click(undo());
  expect(scene().panorama).toBeUndefined();
});
it.each(["epoch", "mode", "readonly"] as const)(
  "cancels a canvas background request when %s changes and ignores late failures",
  async (scope) => {
    const { client, changeScope } = mount();
    const pending = pendingPreview();
    openCanvasBackground();
    fireEvent.click(screen.getByRole("button", { name: "背景甲" }));
    changeScope(scope);
    await act(async () => pending.reject(new Error("过期请求失败")));
    expect(client.changeDirector).not.toHaveBeenCalled();
    expect(screen.queryByText("过期请求失败")).toBeNull();
    expect(previewSignal()?.aborted).toBe(true);
  },
);
it("cancels canvas background loading on unmount", async () => {
  const { client, unmount } = mount();
  const pending = pendingPreview();
  openCanvasBackground();
  fireEvent.click(screen.getByRole("button", { name: "背景甲" }));
  unmount();
  await act(async () => pending.resolve({ asset: imageAssets[0] }));
  expect(client.changeDirector).not.toHaveBeenCalled();
  expect(previewSignal()?.aborted).toBe(true);
});
it("keeps an active canvas background failure retryable", async () => {
  const { client, scene } = mount();
  const first = pendingPreview();
  openCanvasBackground();
  fireEvent.click(screen.getByRole("button", { name: "背景甲" }));
  await act(async () => first.reject(new Error("图片读取失败")));
  expect(screen.getByText("图片读取失败")).toBeTruthy();
  const retry = pendingPreview();
  fireEvent.click(screen.getByRole("button", { name: "背景甲" }));
  await act(async () => retry.resolve({ asset: imageAssets[0] }));
  expect(client.changeDirector).toHaveBeenCalledOnce();
  expect(scene().panorama?.asset_id).toBe(imageAssets[0].id);
});
