import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CanvasNodeType, type CanvasNodeData } from "../model";
import { NodeContent } from "./node-content";

const { preview } = vi.hoisted(() => ({ preview: vi.fn() }));
vi.mock("../queries", () => ({ getMediaPreview: preview }));
class VisibleObserver {
  static instances: VisibleObserver[] = [];
  constructor(readonly callback: IntersectionObserverCallback) {
    VisibleObserver.instances.push(this);
  }
  observe = vi.fn(() => this.show(true));
  disconnect = vi.fn();
  show(isIntersecting: boolean) {
    this.callback(
      [{ isIntersecting } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    );
  }
}
const node: CanvasNodeData = {
  id: "owned-node",
  type: CanvasNodeType.Image,
  title: "Owned image",
  assetId: "owned-asset",
  position: { x: 0, y: 0 },
  width: 160,
  height: 90,
  zIndex: 1,
};
const clients: QueryClient[] = [];
function view(
  props: Partial<React.ComponentProps<typeof NodeContent>> = {},
  strict = false,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  const wrap = (values: Partial<React.ComponentProps<typeof NodeContent>>) => {
    const content = (
      <QueryClientProvider client={client}>
        <NodeContent node={node} projectId="owned-project" active {...values} />
      </QueryClientProvider>
    );
    return strict ? <StrictMode>{content}</StrictMode> : content;
  };
  const result = render(wrap(props));
  return {
    ...result,
    rerenderNode: (values: Partial<React.ComponentProps<typeof NodeContent>>) =>
      result.rerender(wrap(values)),
  };
}
beforeEach(() => {
  VisibleObserver.instances = [];
  preview.mockReset().mockResolvedValue({
    url: "https://media.example/owned",
    expires_at: new Date(Date.now() + 600000).toISOString(),
  });
  vi.stubGlobal("IntersectionObserver", VisibleObserver);
  vi.stubGlobal("matchMedia", () => ({ matches: false }));
  vi.spyOn(document, "hidden", "get").mockReturnValue(false);
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
describe("授权媒体节点生命周期", () => {
  it.each([CanvasNodeType.Image, CanvasNodeType.Video, CanvasNodeType.Audio])(
    "StrictMode重复setup后%s仍挂有效src",
    async (type) => {
      const page = view({ node: { ...node, type }, playable: true }, true);
      await waitFor(() =>
        expect(
          page.container.querySelector(type === "image" ? "img" : type),
        ).not.toBeNull(),
      );
      const media = page.container.querySelector(
        type === "image" ? "img" : type,
      )!;
      expect(media.getAttribute("src")).toBe("https://media.example/owned");
      expect(media.getAttribute("crossorigin")).toBe("anonymous");
    },
  );
  it("图片匿名读取，离屏/隐藏/运动后卸载", async () => {
    const page = view();
    const image = await screen.findByAltText("Owned image");
    expect(image.getAttribute("crossorigin")).toBe("anonymous");
    act(() => VisibleObserver.instances.at(-1)!.show(false));
    expect(screen.queryByAltText("Owned image")).toBeNull();
    act(() => VisibleObserver.instances.at(-1)!.show(true));
    await screen.findByAltText("Owned image");
    page.rerenderNode({ inMotion: true });
    expect(screen.queryByAltText("Owned image")).toBeNull();
    page.rerenderNode({ inMotion: false });
    await screen.findByAltText("Owned image");
    vi.spyOn(document, "hidden", "get").mockReturnValue(true);
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    expect(screen.queryByAltText("Owned image")).toBeNull();
  });
  it.each([CanvasNodeType.Video, CanvasNodeType.Audio])(
    "%s 只有单选playable时挂src，取消后pause/load释放",
    async (type) => {
      const mediaNode = { ...node, type };
      const page = view({ node: mediaNode, playable: false });
      expect(page.container.querySelector(type)).toBeNull();
      expect(preview).not.toHaveBeenCalled();
      page.rerenderNode({ node: mediaNode, playable: true });
      await waitFor(() =>
        expect(page.container.querySelector(type)).not.toBeNull(),
      );
      const media = page.container.querySelector(type) as HTMLMediaElement;
      expect(media.crossOrigin).toBe("anonymous");
      expect(media.hasAttribute("controls")).toBe(true);
      page.rerenderNode({ node: mediaNode, playable: false });
      expect(page.container.querySelector(type)).toBeNull();
      expect(media.hasAttribute("src")).toBe(false);
      expect(media.pause).toHaveBeenCalled();
      expect(media.load).toHaveBeenCalled();
    },
  );
  it("媒体失败只自动重取一次，之后需显式retry", async () => {
    view();
    fireEvent.error(await screen.findByAltText("Owned image"));
    await waitFor(() => expect(preview).toHaveBeenCalledTimes(2));
    fireEvent.error(await screen.findByAltText("Owned image"));
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(preview).toHaveBeenCalledTimes(2);
    fireEvent.click(screen.getByRole("button", { name: "重新读取媒体" }));
    await screen.findByAltText("Owned image");
    expect(preview).toHaveBeenCalledTimes(3);
  });
  it("LOD收起后取消pending授权请求，晚响应不挂src", async () => {
    let signal: AbortSignal | undefined;
    let finish: (value: unknown) => void = () => {};
    preview.mockImplementation((_project, _asset, current: AbortSignal) => {
      signal = current;
      return new Promise((resolve) => {
        finish = resolve;
      });
    });
    const page = view();
    await waitFor(() => expect(preview).toHaveBeenCalledTimes(1));
    page.rerenderNode({ active: false });
    await waitFor(() => expect(signal?.aborted).toBe(true));
    await act(async () =>
      finish({
        url: "https://media.example/late",
        expires_at: new Date(Date.now() + 600000).toISOString(),
      }),
    );
    expect(screen.queryByAltText("Owned image")).toBeNull();
  });
  it("已经过期的URL最多自动renew一次，不循环请求", async () => {
    vi.useFakeTimers();
    preview.mockResolvedValue({
      url: "https://media.example/expired",
      expires_at: new Date(Date.now() - 1000).toISOString(),
    });
    view();
    for (let i = 0; i < 10; i++)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(20);
      });
    expect(preview).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("alert")).toBeTruthy();
  });
});
