import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ShotDetailPage } from "./shot-pages";
const { replace, search } = vi.hoisted(() => ({
  replace: vi.fn(),
  search: { value: "" },
}));
vi.mock("next/navigation", () => ({
  usePathname: () => "/projects/harbor/episodes/ep-01/shots/shot-01",
  useRouter: () => ({ replace }),
  useSearchParams: () => new URLSearchParams(search.value),
}));
afterEach(() => {
  cleanup();
  search.value = "";
  vi.clearAllMocks();
});
it("keeps candidate selection as an impact preview and edits only a local draft", () => {
  search.value = "candidate=b";
  render(
    <ShotDetailPage projectId="harbor" episodeId="ep-01" shotId="shot-01" />,
  );
  expect(
    screen.getByRole("button", { name: "候选 B" }).getAttribute("aria-pressed"),
  ).toBe("true");
  fireEvent.change(screen.getByLabelText("画面描述"), {
    target: { value: "新的构图" },
  });
  expect(screen.getByText(/草稿已修改/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "预览选定影响" }));
  expect(screen.getByRole("dialog", { name: "候选选定影响" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "仅在本页预览选定" }));
  expect(screen.getByText(/本页拟选定：候选 B/)).toBeTruthy();
});
it("blocks draft mutation and selection in readonly mode while preserving comparison", () => {
  search.value = "state=readonly";
  render(
    <ShotDetailPage projectId="harbor" episodeId="ep-01" shotId="shot-01" />,
  );
  expect(
    (screen.getByLabelText("画面描述") as HTMLTextAreaElement).disabled,
  ).toBe(true);
  expect(
    (screen.getByRole("button", { name: "预览选定影响" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "候选 B" }));
  expect(replace).toHaveBeenCalledWith(
    "/projects/harbor/episodes/ep-01/shots/shot-01?state=readonly&candidate=b",
    { scroll: false },
  );
});

it("preserves the candidate when retrying a failed read", async () => {
  const { PreviewBoundary } = await import("./workbench-components");
  search.value = "state=error&candidate=b";
  render(
    <PreviewBoundary>
      <p>任务事实保持不变</p>
    </PreviewBoundary>,
  );
  expect(
    screen.getByRole("heading", { name: "页面读取失败", level: 1 }),
  ).toBeTruthy();
  expect(screen.queryByText("任务事实保持不变")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  expect(replace).toHaveBeenCalledWith(
    "/projects/harbor/episodes/ep-01/shots/shot-01?candidate=b",
    { scroll: false },
  );
});

it("falls back to the normal preview for an unsupported state parameter", async () => {
  const { PreviewBoundary } = await import("./workbench-components");
  search.value = "state=constructor";
  render(
    <PreviewBoundary>
      <p>正常页面样例</p>
    </PreviewBoundary>,
  );
  expect(screen.getByText("正常页面样例")).toBeTruthy();
});
