import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { DirectorProjectAssets } from "@/components/canvas/director/director-project-assets";
const mocks = vi.hoisted(() => ({ list: vi.fn(), preview: vi.fn(), scope: 0 }));
vi.mock("@/api/media", () => ({
  listMediaLibrary: mocks.list,
  getMediaPreview: mocks.preview,
}));
vi.mock("@/lib/request", () => ({
  getScopeGeneration: () => mocks.scope,
  getSessionGeneration: () => 0,
  RequestError: class extends Error {},
}));
vi.mock("@/components/canvas/director/director-asset-preview", () => ({
  DirectorProjectAssetPreview: () => <span>真实预览入口</span>,
}));
const projectID = "10000000-0000-4000-8000-000000000001";
const asset = {
  id: "10000000-0000-4000-8000-000000000002",
  project_id: projectID,
  kind: "model",
  revision: 1,
  file_name: "场景模型.glb",
};
function page(items = [asset], index = 1, total = items.length) {
  return {
    current_actor_id: projectID,
    current_org_id: projectID,
    library_id: projectID,
    revision: 1,
    scope: { kind: "project", project_id: projectID },
    page: index,
    page_size: 40,
    total,
    items: items.map((media) => ({
      id: media.id,
      asset_id: media.id,
      kind: "model",
      catalog_state: "active",
      media,
    })),
  };
}
function mount() {
  const props = {
    projectID,
    kind: "model" as const,
    objectKind: "actor" as const,
    search: "",
    guard: vi.fn(),
    onSelect: vi.fn(),
  };
  return { ...render(<DirectorProjectAssets {...props} />), props };
}
beforeEach(() => {
  mocks.scope = 0;
  mocks.list.mockResolvedValue(page());
  mocks.preview.mockResolvedValue({ asset });
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("loads project-scoped media and verifies the current receipt before handing off placement", async () => {
  const { props } = mount();
  expect(screen.getByRole("status").textContent).toContain("加载");
  fireEvent.click(await screen.findByRole("button", { name: /场景模型/ }));
  await waitFor(() =>
    expect(props.onSelect).toHaveBeenCalledWith([asset], "actor"),
  );
  expect(props.guard).toHaveBeenCalledTimes(3);
  expect(mocks.list.mock.calls[0][0]).toMatchObject({
    scope: "project",
    project_id: projectID,
    kind: "model",
    page: 1,
    page_size: 40,
  });
});
it("shows empty and failed states and retries without inventing local assets", async () => {
  mocks.list
    .mockRejectedValueOnce(new Error("网络暂不可用"))
    .mockResolvedValue(page([]));
  mount();
  expect((await screen.findByRole("alert")).textContent).toContain(
    "网络暂不可用",
  );
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  expect(
    await screen.findByText("暂无模型，可导入本地模型或从项目选择。"),
  ).toBeTruthy();
});
it("deduplicates later pages and preserves the server page progression", async () => {
  mocks.list.mockResolvedValueOnce(page([asset], 1, 41)).mockResolvedValueOnce(
    page(
      [
        asset,
        {
          ...asset,
          id: "10000000-0000-4000-8000-000000000003",
          file_name: "第二模型",
        },
      ],
      2,
      41,
    ),
  );
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "加载更多" }));
  await screen.findByRole("button", { name: /第二模型/ });
  expect(screen.getAllByText("真实预览入口")).toHaveLength(2);
  expect(mocks.list.mock.calls[1][0].page).toBe(2);
});
it("rejects changed asset revisions before placement", async () => {
  mocks.preview.mockResolvedValue({ asset: { ...asset, revision: 2 } });
  const { props } = mount();
  fireEvent.click(await screen.findByRole("button", { name: /场景模型/ }));
  await screen.findByRole("alert");
  expect(props.onSelect).not.toHaveBeenCalled();
});
it("aborts and discards a late selection receipt after unmount", async () => {
  let resolve!: (value: unknown) => void;
  mocks.preview.mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  const { unmount, props } = mount();
  fireEvent.click(await screen.findByRole("button", { name: /场景模型/ }));
  await waitFor(() => expect(mocks.preview).toHaveBeenCalled());
  const signal = mocks.preview.mock.calls[0][1].signal;
  unmount();
  expect(signal.aborted).toBe(true);
  await act(async () => resolve({ asset }));
  expect(props.onSelect).not.toHaveBeenCalled();
});
it("ignores a response after account or organization scope changes", async () => {
  let resolve!: (value: unknown) => void;
  mocks.preview.mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  const { props } = mount();
  fireEvent.click(await screen.findByRole("button", { name: /场景模型/ }));
  await waitFor(() => expect(mocks.preview).toHaveBeenCalled());
  mocks.scope++;
  await act(async () => resolve({ asset }));
  expect(props.onSelect).not.toHaveBeenCalled();
});
