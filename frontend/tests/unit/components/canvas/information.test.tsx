import { afterEach, beforeEach, it, expect, vi } from "vitest";
import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { CanvasNodeSummary } from "@/components/canvas/controls/canvas-node-summary";
import { CanvasInformationDialog } from "@/components/canvas/information/canvas-information-dialog";
import {
  canvasNodeStatistics,
  formatCanvasCharge,
} from "@/components/canvas/information/node-statistics";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
const query = vi.hoisted(() => vi.fn());
vi.mock("@/api/canvases", () => ({ getCanvasInformation: query }));
vi.mock("next/dynamic", () => ({ default: () => () => null }));
const id = "00000000-0000-4000-8000-000000000001",
  project = "00000000-0000-4000-8000-000000000002";
const fixture = () => ({
  id,
  project_id: project,
  owner_name: "项目所有者",
  created_at: "2026-10-22T11:48:00Z",
  updated_at: "2026-10-29T02:00:00Z",
  usage: {
    currency: "CNY",
    settled_micros: "1000001",
    task_count: "1",
    entries: [
      {
        id: "ledger",
        operation_id: "operation",
        created_at: "2026-10-29T02:00:00Z",
        amount_micros: "1000001",
        model_key: "真实模型",
      },
    ],
    next_page: null,
  },
});
afterEach(async () => {
  cleanup();
  await new Promise((resolve) => setTimeout(resolve, 5));
});
beforeEach(() => {
  query.mockReset();
  query.mockResolvedValue(fixture());
});
const nodes = (types: string[]) =>
  types.map((type, index) => ({ id: String(index), type })) as CanvasNodeData[];
it("counts the full graph in source order, including hidden members and external nodes", () => {
  const values = canvasNodeStatistics(
    nodes([
      "group",
      "text",
      "text",
      "director",
      "cinema_studio",
      "image",
      "unsupported",
    ]),
  );
  expect(values.map(({ type, count }) => [type, count])).toEqual([
    ["image", 1],
    ["video", 0],
    ["audio", 0],
    ["text", 2],
    ["timeline", 0],
    ["subject", 0],
    ["external", 2],
    ["other", 2],
  ]);
});
it("opens a 200px summary with nonzero categories only and a real information entry", () => {
  render(
    <CanvasNodeSummary
      nodes={nodes(["text", "text", "video", "director"])}
      canvasID={id}
      projectID={project}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "节点统计，共 4 个节点" }),
  );
  expect(screen.getByText("视频")).toBeTruthy();
  expect(screen.getByText("外部")).toBeTruthy();
  expect(screen.queryByText("图片")).toBeNull();
  expect(screen.getByRole("button", { name: "查看项目信息" })).toBeTruthy();
  expect(screen.getByRole("dialog").className).toContain("w-[200px]");
});
it("formats exact micros without precision loss", () => {
  expect(formatCanvasCharge("0")).toBe("¥0");
  expect(formatCanvasCharge("1000001")).toBe("¥1.000001");
  expect(formatCanvasCharge("9007199254740993")).toBe("¥9,007,199,254.740993");
  expect(() => formatCanvasCharge("NaN")).toThrow();
});
it("reads owner and dates and opens actual fee details without inventing points", async () => {
  render(
    <CanvasInformationDialog
      canvasID={id}
      projectID={project}
      nodes={nodes(["text", "director"])}
      onClose={vi.fn()}
    />,
  );
  await screen.findByText("项目所有者");
  expect(query).toHaveBeenCalledWith(
    { id, page: 1 },
    expect.objectContaining({ signal: expect.any(AbortSignal) }),
  );
  fireEvent.click(screen.getByRole("button", { name: "查看积分明细" }));
  await screen.findByText("实际费用（人民币）");
  expect(screen.getAllByText("¥1.000001")).toHaveLength(2);
  expect(screen.getByText("真实模型")).toBeTruthy();
  expect(screen.getByText(/未配置积分兑换/)).toBeTruthy();
});
it("shows failure rather than a fake zero and retries", async () => {
  query.mockRejectedValueOnce(new Error("request failed"));
  render(
    <CanvasInformationDialog
      canvasID={id}
      projectID={project}
      nodes={[]}
      onClose={vi.fn()}
    />,
  );
  await screen.findByRole("alert");
  expect(screen.queryByText("¥0")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  await screen.findByText("项目所有者");
  expect(query).toHaveBeenCalledTimes(2);
});
it("rejects a mismatched project and aborts requests on unmount", async () => {
  query.mockResolvedValueOnce({ ...fixture(), project_id: "other" });
  const view = render(
    <CanvasInformationDialog
      canvasID={id}
      projectID={project}
      nodes={[]}
      onClose={vi.fn()}
    />,
  );
  await screen.findByRole("alert");
  expect(screen.queryByText("项目所有者")).toBeNull();
  const signal = query.mock.calls[0][1].signal;
  view.unmount();
  expect(signal.aborted).toBe(true);
});
it("paginates through authoritative ledger entries", async () => {
  query
    .mockResolvedValueOnce({
      ...fixture(),
      usage: { ...fixture().usage, next_page: 2 },
    })
    .mockResolvedValueOnce({
      ...fixture(),
      usage: {
        ...fixture().usage,
        entries: [
          {
            ...fixture().usage.entries[0],
            id: "second",
            model_key: "第二页模型",
          },
        ],
      },
    });
  render(
    <CanvasInformationDialog
      canvasID={id}
      projectID={project}
      nodes={[]}
      onClose={vi.fn()}
    />,
  );
  await screen.findByText("项目所有者");
  fireEvent.click(screen.getByRole("button", { name: "查看积分明细" }));
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await screen.findByText("第二页模型");
  await waitFor(() => expect(query.mock.calls[1][0]).toEqual({ id, page: 2 }));
});
