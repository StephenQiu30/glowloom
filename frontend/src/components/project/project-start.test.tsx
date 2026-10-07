import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ProjectStart } from "./project-start";
afterEach(cleanup);
it("无名称不进入创建流程，并把焦点放回错误字段", () => {
  const onStart = vi.fn();
  render(<ProjectStart onStart={onStart} />);
  fireEvent.click(screen.getByRole("button", { name: "创建项目" }));
  expect(onStart).not.toHaveBeenCalled();
  expect(screen.getByRole("alert").textContent).toContain("1–50");
  expect(document.activeElement).toBe(screen.getByLabelText("项目名称"));
});
it("导入与空白项目区分下一步，并保留用户选择的画幅", () => {
  const onStart = vi.fn();
  render(<ProjectStart onStart={onStart} />);
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: " 海边来信 " },
  });
  fireEvent.click(screen.getByRole("radio", { name: "16:9" }));
  fireEvent.click(screen.getByRole("button", { name: /导入本地剧本/ }));
  expect(onStart).toHaveBeenLastCalledWith(
    expect.objectContaining({ name: "海边来信", aspect_ratio: "16:9" }),
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "空白项目" }));
  expect(onStart).toHaveBeenLastCalledWith(
    expect.objectContaining({ name: "海边来信", aspect_ratio: "16:9" }),
    false,
  );
});
