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

it("首页卡片使用真实画布链接，限制三个最近项目并保留新建入口", async () => {
  vi.mocked(listProjects).mockResolvedValue({
    items: [1, 2, 3, 4].map((n) => ({
      ...project,
      id: `11111111-1111-4111-8111-11111111111${n}`,
      name: `画布 ${n}`,
    })),
    next_cursor: null,
  } as unknown as Awaited<ReturnType<typeof listProjects>>);
  render(
    <RecentCanvases presentation="cards">
      <button>新建画布</button>
    </RecentCanvases>,
  );
  expect(await screen.findByRole("link", { name: "画布 1" })).toHaveProperty(
    "href",
    expect.stringContaining("canvas_id="),
  );
  expect(screen.getAllByRole("link")).toHaveLength(3);
  expect(screen.queryByRole("link", { name: "画布 4" })).toBeNull();
  expect(screen.getByRole("button", { name: "新建画布" })).toBeTruthy();
});

it("首页读取失败在卡片区域显示重试，不伪装成空列表", async () => {
  vi.mocked(listProjects)
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce({
      items: [],
      next_cursor: null,
    } as unknown as Awaited<ReturnType<typeof listProjects>>);
  render(<RecentCanvases presentation="cards" />);
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    expect.stringContaining("暂时无法读取画布"),
  );
  fireEvent.click(screen.getByRole("button", { name: "重试读取画布" }));
  expect(await screen.findByRole("heading", { name: "暂无画布" })).toBeTruthy();
});

it("侧栏仅显示五个真实最近画布，更多内容通过外部全部入口访问", async () => {
  vi.mocked(listProjects).mockResolvedValue({
    items: Array.from({ length: 7 }, (_, index) => ({
      ...project,
      id: `11111111-1111-4111-8111-11111111111${index + 1}`,
      name: `最近画布 ${index + 1}`,
    })),
    next_cursor: "server-cursor",
  } as unknown as Awaited<ReturnType<typeof listProjects>>);
  render(<RecentCanvases />);
  await screen.findByRole("link", { name: "最近画布 1" });
  expect(screen.getAllByRole("link").map((link) => link.textContent)).toEqual([
    "最近画布 1",
    "最近画布 2",
    "最近画布 3",
    "最近画布 4",
    "最近画布 5",
  ]);
  expect(screen.queryByRole("button", { name: /加载更多/ })).toBeNull();
  expect(listProjects).toHaveBeenCalledTimes(1);
});
