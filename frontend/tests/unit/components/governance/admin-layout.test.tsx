import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AdminNavigation } from "@/components/governance/admin-layout";
import { AdminTableFilters } from "@/components/governance/admin-table-filters";

afterEach(cleanup);

it("管理导航在日志与审计之间保留全部管理入口及当前项语义", () => {
  const { rerender } = render(<AdminNavigation active="health" />);
  expect(screen.getAllByRole("link")).toHaveLength(7);
  expect(
    screen.getByRole("link", { name: "运行诊断" }).getAttribute("aria-current"),
  ).toBe("page");
  expect(
    screen.getByRole("link", { name: "用户管理" }).getAttribute("href"),
  ).toBe("/users");
  expect(
    screen.getByRole("link", { name: "通知管理" }).getAttribute("href"),
  ).toBe("/platform/notifications");
  expect(
    screen.getByRole("link", { name: "AI 服务" }).getAttribute("href"),
  ).toBe("/models");
  rerender(<AdminNavigation active="audit" />);
  expect(
    screen.getByRole("link", { name: "审计日志" }).getAttribute("aria-current"),
  ).toBe("page");
  expect(
    screen.getByRole("link", { name: "运行诊断" }).getAttribute("aria-current"),
  ).toBeNull();
});

it("折叠筛选保留未提交内容，回车提交条件并支持恢复默认", () => {
  const apply = vi.fn();
  render(
    <AdminTableFilters
      label="模型筛选"
      fields={[
        { name: "search", label: "名称", type: "text" },
        {
          name: "enabled",
          label: "状态",
          type: "select",
          options: [{ value: "true", label: "已启用" }],
        },
      ]}
      onApply={apply}
    />,
  );
  const trigger = screen.getByRole("button", { name: "筛选条件" });
  expect(trigger.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(trigger);
  fireEvent.change(screen.getByLabelText("名称"), {
    target: { value: "  模型一  " },
  });
  fireEvent.click(trigger);
  fireEvent.click(trigger);
  expect(screen.getByLabelText("名称")).toHaveProperty("value", "  模型一  ");
  fireEvent.submit(screen.getByRole("form", { name: "模型筛选" }));
  expect(apply).toHaveBeenLastCalledWith({ search: "模型一" });
  expect(trigger.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("button", { name: "重置筛选" }));
  expect(apply).toHaveBeenLastCalledWith({});
  expect(screen.getByLabelText("名称")).toHaveProperty("value", "");
});
