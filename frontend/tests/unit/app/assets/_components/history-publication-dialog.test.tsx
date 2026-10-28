import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../feedback";
import { HistoryPublicationDialog } from "@/app/assets/_components/history-publication-dialog";
import { historyCardTargets } from "@/app/assets/_components/history-results";
import { requirePersonalHistory } from "@/app/assets/_components/asset-query";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({ publish: vi.fn() }));
vi.mock("@/api/operations", () => ({ publishHistoryResults: api.publish }));
afterEach(cleanup);
beforeEach(() => vi.clearAllMocks());
const id = "11111111-1111-4111-8111-111111111111";
const pid = "22222222-2222-4222-8222-222222222222";
const work = "33333333-3333-4333-8333-333333333333";
function cards(count: number) {
  const task = {
    id,
    project_id: pid,
    capability: "image.generate",
    status: "completed",
    create_time: "2026-10-28T00:00:00Z",
    prompt_preview: "发布测试",
    history_revision: 1,
    outputs: Array.from({ length: count }, (_, index) => ({
      id: `44444444-4444-4444-8444-${String(index + 1).padStart(12, "0")}`,
      kind: "image",
      media_asset_id: pid,
      moderation_status: "passed",
      favorite: false,
      metadata_revision: 1,
      resolution: "2K",
      ratio: "1:1",
      generation_mode: "",
      process_types: [],
      can_download: true,
      can_publish: true,
    })),
  };
  return historyCardTargets(
    requirePersonalHistory({
      items: [task],
      next_cursor: null,
    } as unknown as API.PersonalHistoryPageResponse).items,
    "image",
  );
}
it("发布先明确公开范围，部分成功只对失败目标重试并展示真实站内链接", async () => {
  const selected = cards(2),
    done = vi.fn();
  api.publish
    .mockImplementationOnce(async (body: API.HistoryPublicationInput) => ({
      items: body.targets!.map((target, index) => ({
        ...target,
        status: index ? "failed" : "published",
        work_id: index ? undefined : work,
        url: "https://invalid.example",
      })),
    }))
    .mockImplementation(async (body: API.HistoryPublicationInput) => ({
      items: body.targets!.map((target) => ({
        ...target,
        status: "published",
        work_id: work,
      })),
    }));
  render(
    <HistoryPublicationDialog
      cards={selected}
      onClose={() => {}}
      onLock={() => {}}
      onPublished={done}
    />,
  );
  expect(
    (screen.getByRole("textbox", { name: "标题" }) as HTMLInputElement).value,
  ).not.toContain(selected[0].task.prompt_preview);
  expect(api.publish).not.toHaveBeenCalled();
  expect(screen.getByLabelText("所选发布结果").textContent).toContain("结果2");
  expect(
    screen
      .getByRole("checkbox", { name: "同时公开提示词" })
      .getAttribute("data-state"),
  ).toBe("unchecked");
  fireEvent.click(screen.getByRole("button", { name: "确认发布" }));
  await screen.findByText("已发布1项，失败1项。");
  expect(
    screen.getByRole("link", { name: "查看已发布作品" }).getAttribute("href"),
  ).toBe(`/works/${work}`);
  expect(done).toHaveBeenCalledWith([selected[1].key]);
  fireEvent.click(screen.getByRole("button", { name: "重试失败项" }));
  await waitFor(() => expect(api.publish).toHaveBeenCalledTimes(2));
  expect(api.publish.mock.calls[1][0].targets).toEqual([
    { operation_id: id, output_id: selected[1].output!.id },
  ]);
  expect(api.publish.mock.calls[0][0].publish_prompt).toBe(false);
});
it("未知发布回执保持标题和命令冻结，重试沿用同键同正文", async () => {
  api.publish
    .mockRejectedValueOnce(new RequestError("unavailable", 503))
    .mockImplementation(async (body) => ({
      items: body.targets.map((target: API.HistoryPublicationTarget) => ({
        ...target,
        status: "published",
        work_id: work,
      })),
    }));
  const lock = vi.fn();
  render(
    <HistoryPublicationDialog
      cards={cards(1)}
      onClose={() => {}}
      onLock={lock}
      onPublished={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "确认发布" }));
  await screen.findByRole("button", { name: "重试原操作" });
  expect(
    screen.getByRole("textbox", { name: "标题" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试原操作" }));
  await screen.findByText("已发布1项，失败0项。");
  expect(api.publish.mock.calls[1][0]).toEqual(api.publish.mock.calls[0][0]);
  expect(api.publish.mock.calls[1][1].headers).toEqual(
    api.publish.mock.calls[0][1].headers,
  );
});
it("较多视频结果按16个分批提交，各批独立幂等且不截断结果", async () => {
  api.publish.mockImplementation(async (body: API.HistoryPublicationInput) => ({
    items: body.targets!.map((target) => ({
      ...target,
      status: "published",
      work_id: work,
    })),
  }));
  render(
    <HistoryPublicationDialog
      cards={cards(17)}
      onClose={() => {}}
      onLock={() => {}}
      onPublished={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "确认发布" }));
  await screen.findByText("已发布17项，失败0项。");
  expect(api.publish.mock.calls.map((call) => call[0].targets.length)).toEqual([
    16, 1,
  ]);
  expect(api.publish.mock.calls[0][1].headers["Idempotency-Key"]).not.toBe(
    api.publish.mock.calls[1][1].headers["Idempotency-Key"],
  );
});

it("后续发布子批未知时只重试该批，保留前批回执", async () => {
  api.publish
    .mockImplementationOnce(async (body: API.HistoryPublicationInput) => ({
      items: body.targets!.map((target) => ({
        ...target,
        status: "published",
        work_id: work,
      })),
    }))
    .mockRejectedValueOnce(new RequestError("unavailable", 503))
    .mockImplementation(async (body: API.HistoryPublicationInput) => ({
      items: body.targets!.map((target) => ({
        ...target,
        status: "published",
        work_id: work,
      })),
    }));
  render(
    <HistoryPublicationDialog
      cards={cards(17)}
      onClose={() => {}}
      onLock={() => {}}
      onPublished={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "确认发布" }));
  await screen.findByRole("button", { name: "重试原操作" });
  expect(api.publish).toHaveBeenCalledTimes(2);
  expect(screen.getByText("已发布16项，失败0项。")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "重试原操作" }));
  await screen.findByText("已发布17项，失败0项。");
  expect(api.publish.mock.calls.map((call) => call[0].targets.length)).toEqual([
    16, 1, 1,
  ]);
  expect(api.publish.mock.calls[1][0]).toEqual(api.publish.mock.calls[2][0]);
  expect(api.publish.mock.calls[1][1].headers).toEqual(
    api.publish.mock.calls[2][1].headers,
  );
});
