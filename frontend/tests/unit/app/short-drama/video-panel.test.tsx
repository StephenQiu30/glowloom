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
const guard = vi.hoisted(() => ({
  flush: vi.fn(),
  project: "11111111-1111-4111-8111-111111111111",
  videos: [
    "11111111-1111-4111-8111-111111111112",
    "11111111-1111-4111-8111-111111111113",
  ],
}));
vi.mock("@/api/media", () => ({
  listMediaAssets: async () => ({
    items: guard.videos.map((id, index) => ({
      id,
      project_id: guard.project,
      kind: "video",
      mime_type: "video/mp4",
      revision: 1,
      duration_ms: 8000,
      byte_size: 1000,
      file_name: `素材${index + 1}.mp4`,
    })),
  }),
}));
vi.mock("next/dynamic", () => ({
  default: () =>
    function Generation({
      onCloseGuard,
      seed,
    }: {
      onCloseGuard: (value: (() => Promise<boolean>) | null) => void;
      seed: {
        composer: { references: Array<{ source: { sourceId: string } }> };
      };
    }) {
      useEffect(() => {
        onCloseGuard(guard.flush);
        return () => onCloseGuard(null);
      }, [onCloseGuard]);
      return (
        <div aria-label="转绘编辑器">
          {seed.composer.references[0]?.source.sourceId}
        </div>
      );
    },
}));
import { VideoPanel } from "@/app/short-drama/video-panel";
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("keeps manual redraw available and preserves its draft when a source switch cannot save", async () => {
  guard.flush.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
  render(
    <SWRConfig value={{ provider: () => new Map() }}>
      <VideoPanel
        project={{
          id: guard.project,
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
        onLock={() => {}}
      />
    </SWRConfig>,
  );
  expect(screen.queryByRole("button", { name: "单镜制作" })).toBeNull();
  const chooser = screen.getByRole("combobox", { name: /转绘源视频/ });
  await screen.findByRole("option", { name: "素材1.mp4" });
  fireEvent.change(chooser, { target: { value: guard.videos[0] } });
  fireEvent.click(screen.getByRole("button", { name: "准备转绘" }));
  await screen.findByText(guard.videos[0]);
  fireEvent.change(chooser, { target: { value: guard.videos[1] } });
  fireEvent.click(screen.getByRole("button", { name: "准备转绘" }));
  await waitFor(() => expect(guard.flush).toHaveBeenCalledTimes(1));
  expect(screen.getByText(guard.videos[0])).toBeTruthy();
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "准备转绘" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "准备转绘" }));
  await screen.findByText(guard.videos[1]);
  expect(guard.flush).toHaveBeenCalledTimes(2);
});
