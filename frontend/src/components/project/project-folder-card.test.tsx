import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ProjectFolderCard } from "./project-folder-card";

const folder = {
  id: "9f3e948c-550b-47c8-8fa2-f2c31c88927c",
  name: "同名目录",
  cover: null,
  cover_unavailable: true,
  project_count: 2,
  revision: 3,
  is_delete: false as const,
  delete_time: null,
  create_time: "2026-10-02T00:00:00Z",
  update_time: "2026-10-02T00:00:00Z",
};
const scope = {
  origin: "http://127.0.0.1:3000",
  actorId: "acafc61c-2bf8-48ba-b6c4-f7a84d95c518",
  orgId: "1a39f9d1-a43c-4d98-8947-901b385d5457",
};
afterEach(cleanup);
it("目录可键盘打开并明确UUID、真实计数与不可用封面，菜单提供完整命令", async () => {
  const open = vi.fn();
  const action = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ProjectFolderCard
        folder={folder}
        scope={scope}
        onOpen={open}
        onAction={action}
      />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "打开目录 同名目录" }));
  expect(open).toHaveBeenCalledWith(folder.id);
  expect(screen.getByText("2 个项目")).toBeTruthy();
  expect(screen.getByText("封面不可用")).toBeTruthy();
  expect(screen.getByText(`目录编号 ${folder.id}`)).toBeTruthy();
  expect(
    screen
      .getByText(`编号 ${folder.id.slice(0, 8)}`)
      .closest("p")
      ?.hasAttribute("aria-label"),
  ).toBe(false);
  fireEvent.keyDown(
    screen.getByRole("button", { name: "同名目录的目录操作" }),
    { key: "Enter" },
  );
  fireEvent.click(await screen.findByRole("menuitem", { name: "修改名称" }));
  expect(action).toHaveBeenCalledWith(folder, "rename");
});
