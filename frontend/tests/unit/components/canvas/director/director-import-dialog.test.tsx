import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import {
  DirectorImportDialog,
  type DirectorImportDialogProps,
} from "@/components/canvas/director/director-import-dialog";

const mocks = vi.hoisted(() => ({
  uploadMediaAsset: vi.fn(),
  convertDirectorModel: vi.fn(),
}));
vi.mock("@/api/media", () => ({ uploadMediaAsset: mocks.uploadMediaAsset }));
vi.mock("@/components/canvas/director/director-model-import", () => ({
  DIRECTOR_MODEL_ACCEPT: ".glb,.gltf,.fbx,.obj",
  convertDirectorModel: mocks.convertDirectorModel,
}));
const projectID = "10000000-0000-4000-8000-000000000001";
const receipt = {
  asset: {
    id: "10000000-0000-4000-8000-000000000002",
    project_id: projectID,
    kind: "model",
  },
};
const converted = new File(["GLB test fixture"], "mesh.glb", {
  type: "model/gltf-binary",
});
beforeEach(() => {
  mocks.convertDirectorModel.mockResolvedValue({
    file: converted,
    meshes: 1,
    triangles: 1,
    animations: [],
  });
  mocks.uploadMediaAsset.mockResolvedValue(receipt);
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
function mount(overrides: Partial<DirectorImportDialogProps> = {}) {
  const props = {
    projectID,
    guard: vi.fn(),
    onAdd: vi.fn(),
    onClose: vi.fn(),
    ...overrides,
  };
  return { props, ...render(<DirectorImportDialog {...props} />) };
}
async function choose() {
  fireEvent.change(screen.getByLabelText("选择模型文件"), {
    target: { files: [new File(["OBJ"], "mesh.obj")] },
  });
  await screen.findByText(/mesh.glb/);
}
async function reviewAndUpload() {
  await choose();
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传并放置" }));
}

it("requires explicit local review and uploads the converted GLB to the opening project", async () => {
  const { props } = mount();
  await choose();
  expect(
    (
      screen.getByRole("button", {
        name: "上传并放置",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  expect(mocks.uploadMediaAsset).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传并放置" }));
  await waitFor(() => expect(props.onClose).toHaveBeenCalledOnce());
  expect(mocks.uploadMediaAsset).toHaveBeenCalledWith(
    { pid: projectID },
    { local_review_confirmed: true },
    converted,
    expect.objectContaining({
      signal: expect.any(AbortSignal),
      headers: { "Idempotency-Key": expect.any(String) },
    }),
  );
  expect(props.onAdd).toHaveBeenCalledWith(receipt.asset);
});

it("blocks an epoch change before upload", async () => {
  const guard = vi.fn();
  mount({ guard });
  await choose();
  guard.mockImplementation(() => {
    throw new Error("画布版本已变化");
  });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传并放置" }));
  await screen.findByRole("alert");
  expect(mocks.uploadMediaAsset).not.toHaveBeenCalled();
});

it("retains the accepted asset when placement handoff fails and retries without another upload", async () => {
  const onAdd = vi
    .fn()
    .mockRejectedValueOnce(new Error("暂时无法放置"))
    .mockResolvedValue(undefined);
  const { props } = mount({ onAdd });
  await reviewAndUpload();
  await screen.findByRole("alert");
  expect(props.onClose).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "重试放置" }));
  await waitFor(() => expect(props.onClose).toHaveBeenCalledOnce());
  expect(mocks.uploadMediaAsset).toHaveBeenCalledOnce();
  expect(onAdd).toHaveBeenNthCalledWith(2, receipt.asset);
});

it("retries an unknown receipt using the same request identity and locks file replacement", async () => {
  mocks.uploadMediaAsset.mockRejectedValueOnce(new Error("network lost"));
  mount();
  await reviewAndUpload();
  await screen.findByRole("alert");
  expect(
    (screen.getByLabelText("选择模型文件") as HTMLInputElement).disabled,
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试原上传" }));
  await waitFor(() => expect(mocks.uploadMediaAsset).toHaveBeenCalledTimes(2));
  expect(mocks.uploadMediaAsset.mock.calls[0][3].headers).toEqual(
    mocks.uploadMediaAsset.mock.calls[1][3].headers,
  );
});

it("rejects cross-project or non-model receipts before adding to a scene", async () => {
  mocks.uploadMediaAsset.mockResolvedValue({
    asset: { ...receipt.asset, kind: "image" },
  });
  const { props } = mount();
  await reviewAndUpload();
  await screen.findByRole("alert");
  expect(props.onAdd).not.toHaveBeenCalled();
});

it("aborts upload on unmount and ignores a late receipt", async () => {
  let finish: (value: typeof receipt) => void = () => {};
  mocks.uploadMediaAsset.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { props, unmount } = mount();
  await reviewAndUpload();
  await waitFor(() => expect(mocks.uploadMediaAsset).toHaveBeenCalledOnce());
  const signal = mocks.uploadMediaAsset.mock.calls[0][3].signal;
  unmount();
  expect(signal.aborted).toBe(true);
  await act(async () => {
    finish(receipt);
  });
  expect(props.onAdd).not.toHaveBeenCalled();
});

it("checks the current project again after an in-flight upload", async () => {
  let finish: (value: typeof receipt) => void = () => {};
  mocks.uploadMediaAsset.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { props, rerender } = mount();
  await reviewAndUpload();
  await waitFor(() => expect(mocks.uploadMediaAsset).toHaveBeenCalledOnce());
  rerender(
    <DirectorImportDialog
      {...props}
      projectID="10000000-0000-4000-8000-000000000099"
    />,
  );
  await act(async () => {
    finish(receipt);
  });
  expect(props.onAdd).not.toHaveBeenCalled();
  expect((await screen.findByRole("alert")).textContent).toContain(
    "项目或编辑权限已变化",
  );
});
