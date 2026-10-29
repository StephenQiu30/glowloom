import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { DirectorProjectAssetPreview } from "@/components/canvas/director/director-asset-preview";
const mocks = vi.hoisted(() => ({ preview: vi.fn(), render: vi.fn() }));
vi.mock("@/api/media", () => ({ getMediaPreview: mocks.preview }));
vi.mock("@/components/canvas/director/director-asset-preview-renderer", () => ({
  renderDirectorAssetThumbnail: mocks.render,
}));
const asset = {
  id: "10000000-0000-4000-8000-000000000002",
  project_id: "10000000-0000-4000-8000-000000000001",
  kind: "model",
  revision: 1,
  file_name: "模型",
};
const receipt = () => ({
  asset,
  url: "https://example.test/model.glb",
  expires_at: new Date(Date.now() + 60000).toISOString(),
});
beforeEach(() => {
  mocks.preview.mockResolvedValue(receipt());
  mocks.render.mockResolvedValue("data:image/png;base64,AA==");
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("renders the authorized GLB as an image rather than assigning a GLB URL to an img", async () => {
  render(
    <DirectorProjectAssetPreview projectID={asset.project_id} asset={asset} />,
  );
  const image = await screen.findByRole("img");
  expect(image.getAttribute("src")).toContain("data:image/png");
  expect(mocks.render).toHaveBeenCalledWith(
    "https://example.test/model.glb",
    expect.any(AbortSignal),
  );
});
it("rejects an expired or changed receipt before loading model bytes and allows retry", async () => {
  mocks.preview.mockResolvedValueOnce({
    ...receipt(),
    asset: { ...asset, revision: 2 },
  });
  render(
    <DirectorProjectAssetPreview projectID={asset.project_id} asset={asset} />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "重试预览" }));
  await screen.findByRole("img");
  expect(mocks.render).toHaveBeenCalledOnce();
});
it("aborts an in-flight thumbnail render and ignores its late result", async () => {
  let resolve!: (value: string) => void;
  mocks.render.mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  const { unmount } = render(
    <DirectorProjectAssetPreview projectID={asset.project_id} asset={asset} />,
  );
  await waitFor(() => expect(mocks.render).toHaveBeenCalled());
  const signal = mocks.render.mock.calls[0][1];
  unmount();
  expect(signal.aborted).toBe(true);
  await act(async () => resolve("data:image/png;base64,AA=="));
  expect(screen.queryByRole("img")).toBeNull();
});
