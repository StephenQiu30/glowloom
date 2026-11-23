import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getCanvasStoryboard, writeCanvasStoryboard } from "@/api/canvases";
import { CanvasStoryboard } from "@/components/canvas/storyboard/canvas-storyboard";
import { RequestError } from "@/lib/request";

vi.mock("@/api/canvases", () => ({
  getCanvasStoryboard: vi.fn(),
  writeCanvasStoryboard: vi.fn(),
}));
beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);
vi.mock("@/components/canvas/storyboard/canvas-storyboard-delivery", () => ({
  CanvasStoryboardDelivery: () => null,
}));

const canvasID = "11111111-1111-4111-8111-111111111100";
const projectID = "11111111-1111-4111-8111-111111111101";
const boardID = "11111111-1111-4111-8111-111111111102";
const first = {
  id: "11111111-1111-4111-8111-111111111103",
  revision: 1,
  title: "渡轮",
  action: "渡轮驶过水面",
  duration_ms: 7000,
};
const second = {
  id: "11111111-1111-4111-8111-111111111104",
  revision: 3,
  title: "船舷",
  action: "乘客的脚步",
  duration_ms: 5000,
};
function view(revision: number, shots = [first, second]) {
  return {
    canvas_id: canvasID,
    project_id: projectID,
    revision,
    board: { id: boardID, shots },
  };
}
function renderBoard(onReference = vi.fn(), writable = true) {
  return render(
    <CanvasStoryboard
      active
      canvasID={canvasID}
      projectID={projectID}
      commitSeq="5"
      writable={writable}
      canReference={writable}
      onDiscuss={vi.fn()}
      onReference={onReference}
    />,
  );
}

describe("画布故事板", () => {
  it("重排后保持原来选中的镜头和动作草稿，加入对话时带入稳定身份", async () => {
    vi.mocked(getCanvasStoryboard).mockResolvedValue(view(5));
    vi.mocked(writeCanvasStoryboard).mockResolvedValue(
      view(6, [second, first]),
    );
    const reference = vi.fn();
    renderBoard(reference);
    fireEvent.click(await screen.findByRole("button", { name: /船舷/ }));
    const action = screen.getByRole("textbox", { name: "动作" });
    fireEvent.change(action, { target: { value: "乘客握住栏杆" } });
    fireEvent.click(screen.getByRole("button", { name: "上移" }));
    await waitFor(() => {
      const list = screen.getByRole("list", { name: "镜头顺序" });
      const [row] = within(list).getAllByRole("listitem");
      expect(
        within(row!)
          .getByRole("button", { name: /船舷/ })
          .getAttribute("aria-expanded"),
      ).toBe("true");
      expect((action as HTMLTextAreaElement).value).toBe("乘客握住栏杆");
    });
    expect(writeCanvasStoryboard).toHaveBeenCalledWith(
      { id: canvasID },
      {
        expected_revision: 5,
        board_id: boardID,
        move: { shot_id: second.id, before_id: first.id },
      },
      expect.objectContaining({
        headers: expect.objectContaining({
          "Idempotency-Key": expect.any(String),
        }),
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "加入对话" }));
    expect(reference).toHaveBeenCalledWith(
      expect.stringContaining(
        `第 1 镜（board_id=${boardID}, shot_id=${second.id}, shot_revision=3）「船舷」`,
      ),
    );
  });

  it("同字段已更新时并排保留草稿和正式值，只有复核后才再次保存", async () => {
    vi.mocked(getCanvasStoryboard)
      .mockResolvedValueOnce(view(5))
      .mockResolvedValueOnce(
        view(6, [first, { ...second, action: "松开栏杆", revision: 4 }]),
      );
    vi.mocked(writeCanvasStoryboard).mockRejectedValueOnce(
      new RequestError("storyboard_revision_conflict", 409),
    );
    renderBoard();
    fireEvent.click(await screen.findByRole("button", { name: /船舷/ }));
    const action = screen.getByRole("textbox", {
      name: "动作",
    }) as HTMLTextAreaElement;
    fireEvent.change(action, { target: { value: "乘客握住栏杆" } });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await screen.findByText("当前版本：松开栏杆");
    expect(action.value).toBe("乘客握住栏杆");
    expect(screen.queryByRole("button", { name: "保存" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "保留我的草稿" }));
    expect(screen.getByRole("button", { name: "保存" })).toBeTruthy();
  });
});
