import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { render, findFeedback } from "../../../feedback";
import { afterEach, expect, it, vi } from "vitest";
import { ResultMediaActions } from "@/components/generation/result-media-actions";
const api = vi.hoisted(() => ({ original: vi.fn(), document: vi.fn() }));
vi.mock("@/api/media", () => ({
  downloadMediaOriginal: api.original,
  downloadDocumentAsset: api.document,
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it("downloads only on explicit request through the project original endpoint", async () => {
  api.original.mockRejectedValue(new Error("permission revoked"));
  render(
    <ResultMediaActions
      projectID="project"
      assetID="asset"
      kind="image"
      completed
    />,
  );
  expect(api.original).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "下载原件" }));
  await waitFor(() =>
    expect(api.original).toHaveBeenCalledWith(
      { pid: "project", asset_id: "asset" },
      { responseType: "blob" },
    ),
  );
  expect(api.document).not.toHaveBeenCalled();
  expect(await findFeedback("下载失败，请重试。")).toBeTruthy();
});

it("never offers an original download while a task is still processing", () => {
  render(
    <ResultMediaActions
      projectID="project"
      assetID="asset"
      kind="image"
      completed={false}
    />,
  );
  expect(screen.queryByRole("button", { name: "下载原件" })).toBeNull();
});
