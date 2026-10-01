import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasNodeType, type CanvasNodeData } from "./model";
import { MediaPreviewDialog } from "./media-preview-dialog";

const { preview } = vi.hoisted(() => ({ preview: vi.fn() }));
vi.mock("./queries", () => ({ getMediaPreview: preview }));
const node: CanvasNodeData = {
  id: "preview-node",
  type: CanvasNodeType.Image,
  title: "本地素材",
  assetId: "asset-id",
  position: { x: 0, y: 0 },
  width: 160,
  height: 90,
  zIndex: 0,
};
const clients: QueryClient[] = [];
const asset = { id: "asset-id", project_id: "project-id", kind: "image" };
function show(type = CanvasNodeType.Image) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <MediaPreviewDialog
        node={{ ...node, type }}
        projectId="project-id"
        onClose={vi.fn()}
      />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  preview.mockReset().mockResolvedValue({
    asset,
    url: "https://media.example/original",
    expires_at: new Date(Date.now() + 600000).toISOString(),
  });
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

it("真实图片解码失败提供有界重新授权，成功重试后替换原来源", async () => {
  show();
  const original = await screen.findByRole("img", { name: node.title });
  fireEvent.error(original);
  expect((await screen.findByRole("alert")).textContent).toContain(
    "素材加载失败",
  );
  expect(preview).toHaveBeenCalledTimes(1);
  preview.mockResolvedValueOnce({
    asset,
    url: "https://media.example/refreshed",
    expires_at: new Date(Date.now() + 600000).toISOString(),
  });
  fireEvent.click(screen.getByRole("button", { name: "重试预览" }));
  await waitFor(() =>
    expect(
      screen.getByRole("img", { name: node.title }).getAttribute("src"),
    ).toBe("https://media.example/refreshed"),
  );
  expect(screen.queryByRole("alert")).toBeNull();
  expect(original.hasAttribute("src")).toBe(false);
});

it("关闭完整视频预览释放播放与授权来源", async () => {
  preview.mockResolvedValueOnce({
    asset: { ...asset, kind: "video" },
    url: "https://media.example/original",
    expires_at: new Date(Date.now() + 600000).toISOString(),
  });
  const view = show(CanvasNodeType.Video);
  await waitFor(() =>
    expect(document.querySelector("video")?.getAttribute("src")).toBe(
      "https://media.example/original",
    ),
  );
  const media = document.querySelector("video")!;
  expect(media.getAttribute("crossorigin")).toBe("anonymous");
  view.unmount();
  expect(HTMLMediaElement.prototype.pause).toHaveBeenCalled();
  expect(HTMLMediaElement.prototype.load).toHaveBeenCalled();
  expect(media.hasAttribute("src")).toBe(false);
});

it("重新授权失败即使查询保留旧数据也不继续装配旧来源", async () => {
  show();
  const original = await screen.findByRole("img", { name: node.title });
  fireEvent.error(original);
  preview.mockRejectedValueOnce(new Error("当前项目不允许读取该素材。"));
  fireEvent.click(await screen.findByRole("button", { name: "重试预览" }));
  await waitFor(() =>
    expect(screen.getByRole("alert").textContent).toContain("不允许读取"),
  );
  expect(screen.queryByRole("img")).toBeNull();
  expect(original.hasAttribute("src")).toBe(false);
});

it("失效授权不会装配成可播放来源，允许用户重新读取", async () => {
  preview.mockResolvedValueOnce({
    asset,
    url: "https://media.example/expired",
    expires_at: new Date(Date.now() - 1000).toISOString(),
  });
  show();
  expect((await screen.findByRole("alert")).textContent).toContain(
    "预览授权已过期",
  );
  expect(screen.queryByRole("img")).toBeNull();
  expect(preview).toHaveBeenCalledTimes(1);
});

it.each([
  {
    asset: { ...asset, project_id: "other-project" },
    url: "https://media.example/source",
  },
  {
    asset: { ...asset, id: "other-asset" },
    url: "https://media.example/source",
  },
  { asset: { ...asset, kind: "video" }, url: "https://media.example/source" },
  { asset, url: "data:image/png;base64,AA==" },
  { asset, url: "https://username:password@media.example/source" },
])("身份或来源不符合当前素材时不装配预览 %j", async (value) => {
  preview.mockResolvedValueOnce({
    ...value,
    expires_at: new Date(Date.now() + 600000).toISOString(),
  });
  show();
  expect(await screen.findByRole("alert")).not.toBeNull();
  expect(screen.queryByRole("img")).toBeNull();
});
