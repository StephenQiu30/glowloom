import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { DataPagination } from "@/components/media/data-pagination";

afterEach(cleanup);

it("游标接口只播报当前页，不虚构总页数", () => {
  const next = vi.fn();
  render(
    <DataPagination
      label="画布分页"
      page={2}
      pageSize={12}
      hasNext
      onPageChange={next}
      onPageSizeChange={vi.fn()}
    />,
  );
  expect(screen.getByRole("status").textContent).toContain("第 2 页");
  expect(screen.getByRole("status").textContent).not.toContain("共");
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  expect(next).toHaveBeenCalledWith(3);
});

it("空列表保留数量选择器，同时禁止前后翻页", () => {
  render(
    <DataPagination
      label="素材分页"
      page={1}
      pageSize={12}
      total={0}
      onPageChange={vi.fn()}
      onPageSizeChange={vi.fn()}
    />,
  );
  expect(screen.getByRole("status").textContent).toContain("0 项");
  expect(
    screen.getByRole("combobox", { name: "素材分页每页数量" }),
  ).toHaveProperty("disabled", false);
  expect(screen.getByRole("button", { name: "上一页" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByRole("button", { name: "下一页" })).toHaveProperty(
    "disabled",
    true,
  );
});

it("根据总数处理最后一页，并在未决操作期间禁用所有分页控制", () => {
  const change = vi.fn();
  const props = {
    label: "素材分页",
    page: 2,
    pageSize: 12 as const,
    total: 13,
    onPageChange: change,
    onPageSizeChange: vi.fn(),
  };
  const view = render(<DataPagination {...props} />);
  expect(screen.getByRole("button", { name: "下一页" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "上一页" }));
  expect(change).toHaveBeenCalledWith(1);
  view.rerender(<DataPagination {...props} disabled />);
  expect(screen.getByRole("button", { name: "上一页" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByRole("combobox")).toHaveProperty("disabled", true);
});
