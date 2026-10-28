import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { SWRConfig } from "swr";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { PublicWork } from "@/app/works/[id]/_components/public-work";

const calls = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock("@/api/works", () => ({ getPublishedWork: calls.get }));
const id = "c9aabf47-8479-409c-99d6-faf0555f7b0f";
const work = {
  id,
  title: "已发布作品",
  description: "明确公开的说明",
  kind: "image",
  created_at: "2026-10-28T08:00:00Z",
  media_url: "https://media.example.test/selected-result.png",
  media_expires_at: "2099-01-01T00:00:00Z",
};
function show() {
  return render(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <PublicWork id={id} />
    </SWRConfig>,
  );
}
afterEach(cleanup);
beforeEach(() => {
  calls.get.mockReset();
});
it("公开页只渲染具体作品，不要求会话恢复或显示未公开提示词", async () => {
  calls.get.mockResolvedValue(work);
  show();
  expect(
    await screen.findByRole("heading", { name: "已发布作品" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("img", { name: "已发布作品" }).getAttribute("src"),
  ).toBe(work.media_url);
  expect(screen.queryByRole("heading", { name: "公开提示词" })).toBeNull();
  expect(calls.get).toHaveBeenCalledWith(
    { work_id: id },
    { sessionRecovery: "never" },
  );
});
it("拒绝错作品或非HTTP媒体地址，媒体失败可重新读取短效地址", async () => {
  calls.get
    .mockResolvedValueOnce({ ...work, media_url: "javascript:alert(1)" })
    .mockResolvedValue(work);
  show();
  expect(
    await screen.findByRole("heading", { name: "作品暂不可用" }),
  ).toBeTruthy();
  expect(screen.queryByRole("img")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  const img = await screen.findByRole("img", { name: "已发布作品" });
  fireEvent.error(img);
  expect(screen.getByRole("heading", { name: "媒体读取失败" })).toBeTruthy();
  calls.get.mockResolvedValue({
    ...work,
    media_url: "https://media.example.test/refreshed.png",
  });
  fireEvent.click(screen.getByRole("button", { name: "重新读取" }));
  await waitFor(() =>
    expect(
      screen.getByRole("img", { name: "已发布作品" }).getAttribute("src"),
    ).toBe("https://media.example.test/refreshed.png"),
  );
});
