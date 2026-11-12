import { useEffect } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { SWRConfig } from "swr";
import { afterEach, expect, it, vi } from "vitest";
const guard = vi.hoisted(() => ({ flush: vi.fn() }));
vi.mock("@/api/media", () => ({
  listMediaAssets: async () => ({ items: [] }),
}));
vi.mock("next/dynamic", () => ({
  default: (loader: () => Promise<unknown>) =>
    loader.toString().includes("rich-generation")
      ? function Generation({
          onCloseGuard,
        }: {
          onCloseGuard: (value: (() => Promise<boolean>) | null) => void;
        }) {
          useEffect(() => {
            onCloseGuard(guard.flush);
            return () => onCloseGuard(null);
          }, [onCloseGuard]);
          return <div aria-label="原转绘编辑器" />;
        }
      : () => <div aria-label="单镜 Agent" />,
}));
import { VideoPanel } from "@/app/short-drama/video-panel";
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("flushes the current redraw draft before switching to the Agent and keeps it mounted if saving is refused", async () => {
  guard.flush.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
  render(
    <SWRConfig value={{ provider: () => new Map() }}>
      <VideoPanel
        project={{
          id: "11111111-1111-4111-8111-111111111111",
          canvas_id: null,
          name: "短剧",
          revision: 1,
          aspect_ratio: "16:9",
          style_key: null,
          update_time: new Date().toISOString(),
        }}
        owner="actor/space"
        canvas="/canvas"
        writable
        initialPrompt="镜头要求"
        onLock={() => {}}
      />
    </SWRConfig>,
  );
  fireEvent.click(screen.getByRole("button", { name: "视频转绘" }));
  await screen.findByLabelText("原转绘编辑器");
  fireEvent.click(screen.getByRole("button", { name: "单镜制作" }));
  await waitFor(() => expect(guard.flush).toHaveBeenCalledTimes(1));
  expect(screen.getByLabelText("原转绘编辑器")).toBeTruthy();
  expect(screen.queryByLabelText("单镜 Agent")).toBeNull();
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "单镜制作" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "单镜制作" }));
  await screen.findByLabelText("单镜 Agent");
  expect(guard.flush).toHaveBeenCalledTimes(2);
});
