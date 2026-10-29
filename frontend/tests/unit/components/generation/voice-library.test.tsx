import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { VoiceLibrary } from "@/components/generation/voice-library";
const calls = vi.hoisted(() => ({
  read: vi.fn(),
  system: vi.fn(),
  save: vi.fn(),
  change: vi.fn(),
  favorite: vi.fn(),
}));
vi.mock("@/components/generation/voice-library-api", () => ({
  readVoices: calls.read,
  readSystemVoices: calls.system,
  saveVoice: calls.save,
  changeVoice: calls.change,
  favoriteSystem: calls.favorite,
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
  ProjectMediaPicker: ({
    onSelect,
  }: {
    onSelect: (v: API.AssetSummary) => void;
  }) => (
    <button
      onClick={() =>
        onSelect({
          id: "asset",
          project_id: "project",
          kind: "audio",
          duration_ms: 1000,
          byte_size: 48044,
          revision: 1,
          file_name: "sample.wav",
        })
      }
    >
      选择测试音频
    </button>
  ),
}));
const voice = {
  id: "voice",
  project_id: "project",
  name: "旁白",
  asset_id: "asset",
  asset_revision: 1,
  revision: 1,
  favorite: false,
  updated_at: "2026-10-29T00:00:00Z",
};
beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  calls.read.mockResolvedValue([]);
  calls.system.mockResolvedValue([
    { id: "speaker", name: "Vivi 2.0", favorite: false },
  ]);
});
afterEach(cleanup);
it("没有模型请求也能收藏系统音色，失败不会伪造收藏", async () => {
  calls.favorite.mockRejectedValueOnce(new Error("保存失败"));
  render(
    <VoiceLibrary
      ownerKey="actor/project"
      projectID="project"
      onClose={() => {}}
      onSelect={() => {}}
    />,
  );
  const favorite = await screen.findByRole("button", { name: "收藏Vivi 2.0" });
  fireEvent.click(favorite);
  await screen.findByText("保存失败");
  expect(favorite.getAttribute("aria-pressed")).toBe("false");
  calls.favorite.mockResolvedValue({});
  fireEvent.click(favorite);
  await screen.findByRole("button", { name: "取消收藏Vivi 2.0" });
});
it("创建超时后锁住输入，按相同请求恢复且显示持久化结果", async () => {
  calls.save
    .mockRejectedValueOnce(new Error("网络中断"))
    .mockResolvedValue(voice);
  const { unmount } = render(
    <VoiceLibrary
      ownerKey="actor/project"
      projectID="project"
      onClose={() => {}}
      onSelect={() => {}}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "创建音色" }));
  fireEvent.change(screen.getByRole("textbox", { name: "音色名称" }), {
    target: { value: "旁白" },
  });
  fireEvent.click(screen.getByRole("button", { name: "从项目选择音频" }));
  fireEvent.click(screen.getByRole("button", { name: "选择测试音频" }));
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "保存音色" }));
  await screen.findByText("网络中断");
  unmount();
  render(
    <VoiceLibrary
      ownerKey="actor/project"
      projectID="project"
      onClose={() => {}}
      onSelect={() => {}}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "创建音色" }));

  expect(
    screen.getByRole("textbox", { name: "音色名称" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "恢复保存结果" }));
  await waitFor(() => expect(calls.save).toHaveBeenCalledTimes(2));
  expect(calls.save.mock.calls[0]).toEqual(calls.save.mock.calls[1]);
  await screen.findByRole("heading", { name: "旁白" });
});
