import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { AssetsWorkspace } from "./assets-workspace";

const ports = vi.hoisted(() => ({
  projects: vi.fn(),
  assets: vi.fn(),
  preview: vi.fn(),
}));
vi.mock("@/components/project/queries", () => ({
  PROJECTS_KEY: ["projects"],
  listProjects: ports.projects,
}));
vi.mock("@/components/canvas/queries", () => ({
  listMediaAssets: ports.assets,
  getMediaPreview: ports.preview,
}));
const projectId = "f61b4c06-bf90-43fd-b5d1-6acd51d28078";
const project = {
  id: projectId,
  name: "逆光",
  status: "active",
  is_delete: false,
};
const image = {
  id: "629e1b9c-06fb-47ca-958e-743f86279826",
  project_id: projectId,
  kind: "image" as const,
  file_name: "角色参考.png",
  mime_type: "image/png",
  byte_size: 10240,
  revision: 1,
};
const video = {
  ...image,
  id: "0b257e62-4317-4c9c-bd52-3f15c7c226b5",
  kind: "video" as const,
  file_name: "逆光镜头.mp4",
  mime_type: "video/mp4",
};
const audio = {
  ...image,
  id: "ee130974-7ef7-4e85-9a72-4e4b4ad6f21d",
  kind: "audio" as const,
  file_name: "旁白.wav",
  mime_type: "audio/wav",
};
const clients: QueryClient[] = [];
function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  const mounted = render(
    <QueryClientProvider client={client}>
      <AssetsWorkspace />
    </QueryClientProvider>,
  );
  return { ...mounted, client };
}
beforeEach(() => {
  vi.resetAllMocks();
  ports.projects.mockResolvedValue({ items: [project], next_cursor: null });
  ports.assets.mockResolvedValue({
    items: [image, video, audio],
    next_cursor: null,
  });
  ports.preview.mockImplementation((_pid, id) => ({
    asset: [image, video, audio].find((item) => item.id === id),
    url: "https://objects.example.test/media?signature=synthetic",
    expires_at: new Date(Date.now() + 60000).toISOString(),
  }));
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
  HTMLElement.prototype.scrollIntoView = vi.fn();
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

it("读取真实活动项目与媒体，不提前获取授权链接或捏造缩略图", async () => {
  const view = setup();
  await screen.findByRole("button", { name: "预览 角色参考.png" });
  expect(ports.projects.mock.calls[0][0]).toEqual({
    limit: 100,
    status: "active",
    deleted: false,
    cursor: undefined,
  });
  expect(ports.assets.mock.calls[0][0]).toBe(projectId);
  expect(ports.preview).not.toHaveBeenCalled();
  expect(view.container.querySelectorAll("img, video, audio")).toHaveLength(0);
  expect(screen.getByText(/在当前页筛选资源/)).toBeTruthy();
});

it("无项目时提供创建入口，不发起无所属项目的媒体读取", async () => {
  ports.projects.mockResolvedValue({ items: [], next_cursor: null });
  setup();
  await screen.findByText("还没有可用的项目");
  expect(
    screen.getByRole("link", { name: "新建项目" }).getAttribute("href"),
  ).toBe("/projects?create=true");
  expect(ports.assets).not.toHaveBeenCalled();
});

it("当前页类型与文件名筛选无结果仍可用真实游标读取下一页", async () => {
  ports.assets
    .mockResolvedValueOnce({ items: [image], next_cursor: "media-page-2" })
    .mockResolvedValue({ items: [video], next_cursor: null });
  setup();
  await screen.findByRole("button", { name: "预览 角色参考.png" });
  fireEvent.click(screen.getByRole("radio", { name: "视频" }));
  fireEvent.change(screen.getByRole("textbox", { name: "搜索当前页资源" }), {
    target: { value: "逆光" },
  });
  expect(screen.getByText("当前页没有匹配的资源")).toBeTruthy();
  expect(ports.assets).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await screen.findByRole("button", { name: "预览 逆光镜头.mp4" });
  expect(ports.assets.mock.calls[1][1]).toBe("media-page-2");
  expect(screen.getByText("第 2 页")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "上一页" }));
  await screen.findByText("第 1 页");
  expect(screen.getByText("当前页没有匹配的资源")).toBeTruthy();
});

it("读取更多真实项目后切换项目，资源分页从头开始", async () => {
  const other = {
    ...project,
    id: "c517f8a5-80c0-4bdb-90fd-bd70dd1c6bc2",
    name: "海边",
  };
  ports.projects
    .mockResolvedValueOnce({ items: [project], next_cursor: "project-page-2" })
    .mockResolvedValue({ items: [other], next_cursor: null });
  ports.assets.mockResolvedValue({
    items: [image],
    next_cursor: "media-page-2",
  });
  setup();
  await screen.findByRole("button", { name: "预览 角色参考.png" });
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await screen.findByText("第 2 页");
  fireEvent.click(screen.getByRole("button", { name: "读取更多项目" }));
  await waitFor(() => expect(ports.projects).toHaveBeenCalledTimes(2));
  fireEvent.keyDown(screen.getByRole("combobox", { name: "资源所属项目" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: "海边" }));
  await waitFor(() =>
    expect(
      ports.assets.mock.calls.some(
        ([id, cursor]) => id === other.id && cursor === undefined,
      ),
    ).toBe(true),
  );
  expect(screen.getByText("第 1 页")).toBeTruthy();
  expect(ports.projects.mock.calls[1][0].cursor).toBe("project-page-2");
});

it("资源服务失败明确展示请求编号，手动重试后展示真实空态", async () => {
  ports.assets.mockRejectedValueOnce(
    new ApiError(503, "dependency_unavailable", "asset-read-1"),
  );
  ports.assets.mockResolvedValue({ items: [], next_cursor: null });
  setup();
  await screen.findByText("资源未能加载");
  expect(screen.getByText("请求编号：asset-read-1")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "重试读取资源" }));
  await screen.findByText("这个项目还没有资源");
  expect(
    screen.getByText("项目已有素材会显示在这里，媒体上传入口正在准备中。"),
  ).toBeTruthy();
  expect(screen.queryByRole("link", { name: /登录/ })).toBeNull();
});

it.each([image, video, audio])(
  "仅点击后预览$kind，关闭释放媒体与短期缓存，重开重新授权",
  async (asset) => {
    const { client, container } = setup();
    const trigger = await screen.findByRole("button", {
      name: `预览 ${asset.file_name}`,
    });
    fireEvent.click(trigger);
    await waitFor(() =>
      expect(
        document.querySelector(asset.kind === "image" ? "img" : asset.kind),
      ).not.toBeNull(),
    );
    const source = document.querySelector(
      asset.kind === "image" ? "img" : asset.kind,
    )!;
    expect(source.getAttribute("src")).toBe(
      "https://objects.example.test/media?signature=synthetic",
    );
    expect(source.getAttribute("crossorigin")).toBe("anonymous");
    if (asset.kind !== "image")
      expect(source.hasAttribute("controls")).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "关闭预览" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(source.hasAttribute("src")).toBe(false);
    await waitFor(() =>
      expect(
        client.getQueryData(["assets", "preview", projectId, asset.id]),
      ).toBeUndefined(),
    );
    expect(container.querySelectorAll("img, video, audio")).toHaveLength(0);
    fireEvent.click(trigger);
    await waitFor(() => expect(ports.preview).toHaveBeenCalledTimes(2));
  },
);

it.each(["javascript:alert(1)", "https://objects.example.test/expired"])(
  "无有效授权的%s不挂载媒体，不自动循环重试",
  async (url) => {
    ports.preview.mockResolvedValue({
      asset: image,
      url,
      expires_at: new Date(Date.now() - 1000).toISOString(),
    });
    setup();
    fireEvent.click(
      await screen.findByRole("button", { name: "预览 角色参考.png" }),
    );
    await screen.findByText("资源暂时无法预览");
    expect(document.querySelector("img")).toBeNull();
    expect(ports.preview).toHaveBeenCalledTimes(1);
    ports.preview.mockResolvedValue({
      asset: image,
      url: "https://objects.example.test/refreshed",
      expires_at: new Date(Date.now() + 60000).toISOString(),
    });
    fireEvent.click(screen.getByRole("button", { name: "重新获取预览" }));
    expect(
      (await screen.findByAltText(image.file_name)).getAttribute("src"),
    ).toBe("https://objects.example.test/refreshed");
  },
);

it("关闭预览取消未完成授权请求，晚返回结果不挂载媒体", async () => {
  let signal: AbortSignal | undefined;
  let finish: (value: unknown) => void = () => {};
  ports.preview.mockImplementation(
    (_pid, _asset, currentSignal: AbortSignal) => {
      signal = currentSignal;
      return new Promise((resolve) => {
        finish = resolve;
      });
    },
  );
  setup();
  fireEvent.click(
    await screen.findByRole("button", { name: "预览 角色参考.png" }),
  );
  await screen.findByText("正在获取授权预览…");
  fireEvent.click(screen.getByRole("button", { name: "关闭预览" }));
  await waitFor(() => expect(signal?.aborted).toBe(true));
  await act(async () =>
    finish({
      asset: image,
      url: "https://objects.example.test/late",
      expires_at: new Date(Date.now() + 60000).toISOString(),
    }),
  );
  expect(document.querySelector("img")).toBeNull();
  expect(screen.queryByRole("dialog")).toBeNull();
});
