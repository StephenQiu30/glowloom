import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasProjectHistory } from "./project-history";
const list = vi.hoisted(() => vi.fn());
vi.mock("@/api/operations", () => ({ listProjectOperations: list }));
afterEach(cleanup);
it.each([
  { id: "task", project_id: "other-project", status: "completed" },
  { id: "task", project_id: "project", status: "toString" },
])("拒绝跨项目或未知状态的历史回执", async (task) => {
  list.mockResolvedValue({ items: [task] });
  render(<CanvasProjectHistory projectID="project" onClose={vi.fn()} />);
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    "生成历史读取失败，请重试。重试",
  );
});
