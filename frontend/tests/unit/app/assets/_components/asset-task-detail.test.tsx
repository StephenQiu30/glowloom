import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TaskDetail } from "@/components/generation/generation-dialog";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock("@/api/operations", () => ({ getOperation: api.get }));
vi.mock("@/components/generation/rich-generation", () => ({
  RichGeneration: () => null,
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
}));
const id = "11111111-1111-4111-8111-111111111111";
const projectID = "22222222-2222-4222-8222-222222222222";
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it("asset detail replaces a failed first read with an inline retry and never keeps a spinner", async () => {
  api.get.mockRejectedValueOnce(new RequestError("unavailable", 503));
  api.get.mockResolvedValueOnce({
    id,
    project_id: projectID,
    status: "completed",
    inputs: [{ role: "prompt", text: "恢复的真实提示词" }],
    outputs: [],
  });
  render(<TaskDetail projectID={projectID} id={id} presentation="asset" />);
  expect(await screen.findByText("暂时无法读取生成记录")).toBeTruthy();
  expect(screen.queryByText("正在读取结果")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试读取详情" }));
  expect(await screen.findByText("恢复的真实提示词")).toBeTruthy();
});

it.each([401, 403, 404])(
  "asset detail clears its private prompt and reuse action when a refresh is denied (%s)",
  async (status) => {
    const reuse = vi.fn();
    api.get.mockResolvedValueOnce({
      id,
      project_id: projectID,
      status: "completed",
      inputs: [{ role: "prompt", text: "私有提示词" }],
      outputs: [],
    });
    api.get.mockRejectedValueOnce(new RequestError("forbidden", status));
    render(
      <TaskDetail
        projectID={projectID}
        id={id}
        presentation="asset"
        onReuse={reuse}
      />,
    );
    await screen.findByText("私有提示词");
    fireEvent.click(screen.getByRole("button", { name: "刷新任务" }));
    expect(await screen.findByText("此生成记录已不可访问")).toBeTruthy();
    expect(screen.queryByText("私有提示词")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "复用参数重新报价" }),
    ).toBeNull();
    expect(reuse).not.toHaveBeenCalled();
  },
);

it("a removed selected output has an explicit state and disables its result actions", async () => {
  const available = vi.fn();
  api.get.mockResolvedValue({
    id,
    project_id: projectID,
    status: "completed",
    outputs: [],
  });
  render(
    <TaskDetail
      projectID={projectID}
      id={id}
      outputID={id}
      presentation="asset"
      onAvailabilityChange={available}
    />,
  );
  expect(await screen.findByText("所选结果已不可用")).toBeTruthy();
  expect(available).toHaveBeenLastCalledWith(false);
});

it("a completed structured candidate without text explains why PDF and Markdown are unavailable", async () => {
  api.get.mockResolvedValue({
    id,
    project_id: projectID,
    status: "completed",
    capability: "text.structured",
    outputs: [{ id, kind: "json", moderation_status: "passed" }],
  });
  render(
    <TaskDetail
      projectID={projectID}
      id={id}
      outputID={id}
      presentation="asset"
    />,
  );
  expect(await screen.findByText("此结果没有可导出的正文")).toBeTruthy();
  expect(
    screen.getByText(
      "该记录未提供文档正文，无法导出PDF或Markdown。你仍可查看生成参数。",
    ),
  ).toBeTruthy();
  expect(screen.queryByText("正在读取结果")).toBeNull();
});

it("an unfinished formal candidate is shown as metadata without implying it can be downloaded", async () => {
  api.get.mockResolvedValue({
    id,
    project_id: projectID,
    status: "failed",
    capability: "image.generate",
    outputs: [
      { id, kind: "document", media_asset_id: id, moderation_status: "passed" },
    ],
  });
  render(
    <TaskDetail
      projectID={projectID}
      id={id}
      outputID={id}
      presentation="asset"
    />,
  );
  expect(
    await screen.findByText("文档结果尚未完成，暂不可下载。"),
  ).toBeTruthy();
  expect(screen.queryByText("暂时无法读取生成记录")).toBeNull();
});
