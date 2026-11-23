import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { beforeEach, expect, it, vi } from "vitest";
import { ProjectMemoryDialog } from "@/components/conversations/project-memory-dialog";

const api = vi.hoisted(() => ({
  listProjectMemory: vi.fn(),
  listProjectMemoryHistory: vi.fn(),
  publishProjectMemory: vi.fn(),
}));
vi.mock("@/api/projectMemory", () => api);

beforeEach(() => {
  vi.resetAllMocks();
  api.listProjectMemoryHistory.mockResolvedValue([]);
});

function open() {
  render(
    <SWRConfig value={{ provider: () => new Map(), shouldRetryOnError: false }}>
      <ProjectMemoryDialog
        projectID="project"
        initialStatement="草拟的私人内容"
        onClose={() => {}}
      />
    </SWRConfig>,
  );
}

it("只提交用户编辑后明确确认的独立约定", async () => {
  api.listProjectMemory.mockResolvedValue({ can_edit: true, items: [] });
  api.publishProjectMemory.mockResolvedValue({ revision: 1 });
  open();
  const content = await screen.findByLabelText("确认共享的内容");
  expect((content as HTMLTextAreaElement).value).toBe("草拟的私人内容");
  expect(api.publishProjectMemory).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("约定名称"), {
    target: { value: "声音" },
  });
  fireEvent.change(content, { target: { value: "全片无旁白" } });
  fireEvent.click(screen.getByRole("button", { name: "确认发布" }));
  await waitFor(() =>
    expect(api.publishProjectMemory).toHaveBeenCalledWith(
      { pid: "project", key: encodeURIComponent("声音") },
      { expected_revision: 0, statement: "全片无旁白" },
      expect.objectContaining({
        headers: expect.objectContaining({
          "Idempotency-Key": expect.any(String),
        }),
      }),
    ),
  );
});

it("只读访问可以查看约定，且不会建立发布请求", async () => {
  api.listProjectMemory.mockResolvedValue({
    can_edit: false,
    items: [{ id: "m1", key: "声音", revision: 2, statement: "保留同期声" }],
  });
  open();
  expect(await screen.findByText("保留同期声")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "确认发布" })).toBeNull();
  expect(api.publishProjectMemory).not.toHaveBeenCalled();
});
