import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ReferenceSearch } from "./reference-search";
const api = vi.hoisted(() => ({ media: vi.fn() }));
vi.mock("@/api/media", () => ({ listMediaAssets: api.media }));
vi.mock("@/api/bible", () => ({
  listBibleEntries: vi.fn(),
  getBibleEntry: vi.fn(),
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("ignores an older authorized search result and selects the current result by keyboard", async () => {
  let release!: (page: API.AssetPage) => void;
  api.media.mockImplementationOnce(
    () =>
      new Promise<API.AssetPage>((resolve) => {
        release = resolve;
      }),
  );
  const chosen = vi.fn();
  render(
    <ReferenceSearch
      projectID="project"
      initialQuery="旧"
      onSelect={chosen}
      onClose={() => {}}
    />,
  );
  await waitFor(() => expect(api.media).toHaveBeenCalledTimes(1));
  api.media.mockResolvedValue({
    items: [
      {
        id: "new",
        project_id: "project",
        kind: "image",
        file_name: "新图片.png",
      },
    ],
  });
  fireEvent.change(screen.getByLabelText("搜索项目引用"), {
    target: { value: "新" },
  });
  await screen.findByRole("option", { name: /新图片/ });
  release({
    items: [
      {
        id: "old",
        project_id: "project",
        kind: "image",
        file_name: "旧图片.png",
      },
    ],
  });
  await waitFor(() =>
    expect(screen.queryByRole("option", { name: /旧图片/ })).toBeNull(),
  );
  fireEvent.keyDown(screen.getByLabelText("搜索项目引用"), { key: "Enter" });
  expect(chosen).toHaveBeenCalledWith([expect.objectContaining({ id: "new" })]);
});
it("rejects a response from a different project", async () => {
  api.media.mockResolvedValue({
    items: [
      {
        id: "other",
        project_id: "private",
        kind: "image",
        file_name: "私有图片",
      },
    ],
  });
  render(
    <ReferenceSearch
      projectID="project"
      initialQuery=""
      onSelect={() => {}}
      onClose={() => {}}
    />,
  );
  await screen.findByRole("alert");
  expect(screen.queryByText("私有图片")).toBeNull();
});
