import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasNodeType } from "./model";
import { VideoToolsDialog } from "./video-tools-dialog";
vi.mock("./queries", () => ({
  getMediaPreview: vi.fn().mockRejectedValue(new Error("合成测试未请求原件")),
}));
const clients: QueryClient[] = [];
beforeEach(() =>
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  ),
);
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((c) => c.clear());
  vi.unstubAllGlobals();
});
function show(disabled: boolean, open: () => void) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <VideoToolsDialog
        node={{
          id: "0c6e50ee-0912-4da2-96b3-c84e27b61389",
          assetId: "fba9de81-80c4-49ce-8f59-1dac39717a04",
          type: CanvasNodeType.Video,
          title: "正式视频",
          position: { x: 0, y: 0 },
          width: 200,
          height: 120,
          zIndex: 0,
        }}
        projectId="b580ad59-17a5-4ed2-985b-cb0cdd04b4c4"
        remainingSlots={10}
        onClose={vi.fn()}
        onFrame={vi.fn()}
        onTimeline={vi.fn()}
        onDepth={open}
        depthDisabled={disabled}
      />
    </QueryClientProvider>,
  );
}
it("正式视频工具入口转到深度流程；未保存/只读时明确禁用", async () => {
  const open = vi.fn();
  show(false, open);
  fireEvent.click(screen.getByRole("button", { name: "生成视频深度" }));
  expect(open).toHaveBeenCalledOnce();
  cleanup();
  show(true, open);
  expect(
    screen
      .getByRole("button", { name: "生成视频深度" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "生成视频深度" }));
  expect(open).toHaveBeenCalledOnce();
});
