import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { RecentCanvases } from "@/components/layout/recent-canvases";
import { listProjects } from "@/api/projects";
vi.mock("@/api/projects", () => ({ listProjects: vi.fn() }));
vi.mock("@/components/projects/project-presentation-store", () => ({
  subscribeProjectInvalidation: () => () => {},
}));
vi.mock("@/components/ui/sidebar", () => ({
  SidebarMenu: (p: object) => <ul {...p} />,
  SidebarMenuItem: (p: object) => <li {...p} />,
  SidebarMenuButton: ({ children }: { children: React.ReactNode }) => children,
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
const project = {
  id: "11111111-1111-4111-8111-111111111111",
  canvas_id: "22222222-2222-4222-8222-222222222222",
  name: "真实画布",
  aspect_ratio: "16:9",
  style_key: null,
  revision: 1,
  update_time: "2026-10-23T00:00:00Z",
};
it("routes to the server canvas and requests active projects in content order", async () => {
  vi.mocked(listProjects).mockResolvedValue({
    items: [project],
    next_cursor: null,
  } as unknown as Awaited<ReturnType<typeof listProjects>>);
  render(<RecentCanvases onNavigate={() => {}} />);
  expect(
    (await screen.findByRole("link", { name: "真实画布" })).getAttribute(
      "href",
    ),
  ).toBe(`/canvas?project_id=${project.id}&canvas_id=${project.canvas_id}`);
  expect(listProjects).toHaveBeenCalledWith(
    { limit: 20, status: "active", order: "recent_content" },
    expect.objectContaining({ signal: expect.any(AbortSignal) }),
  );
});
it("does not disguise a rejected request as an empty history and can retry", async () => {
  vi.mocked(listProjects)
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce({
      items: [],
      next_cursor: null,
    } as unknown as Awaited<ReturnType<typeof listProjects>>);
  render(<RecentCanvases onNavigate={() => {}} />);
  fireEvent.click(await screen.findByRole("button", { name: "重试读取画布" }));
  await waitFor(() => expect(screen.getByText("暂无画布")).toBeTruthy());
});
