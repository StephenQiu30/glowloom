import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SubjectDateRange } from "@/components/subjects/subject-date-range";
import { UnifiedSubjectToolbar } from "@/components/subjects/unified-toolbar";

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(2026, 9, 28, 12));
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
function setup(from = "", through = "") {
  const onChange = vi.fn();
  render(
    <SubjectDateRange
      from={from}
      through={through}
      disabled={false}
      onChange={onChange}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "时间" }));
  return onChange;
}
function custom() {
  fireEvent.click(screen.getByRole("button", { name: /开始日期.*结束日期/ }));
}
function day(value: string) {
  return screen.getByRole("button", { name: value });
}

it("用日历选择闭合区间，保留起止选中和中间范围，应用前不修改筛选", async () => {
  const change = setup();
  custom();
  expect(document.querySelector('input[type="date"]')).toBeNull();
  fireEvent.click(day("2026年10月8日"));
  expect(screen.getByText("请选择结束日期")).toBeTruthy();
  fireEvent.click(day("2026年10月12日"));
  expect(
    screen.getByText("已选择 2026-10-08 至 2026-10-12，可应用时间范围。"),
  ).toBeTruthy();
  expect(screen.queryByText("请选择结束日期")).toBeNull();
  expect(screen.getByRole("button", { name: "开始日期" }).textContent).toBe(
    "2026-10-08",
  );
  expect(
    day("2026年10月10日").closest("td")?.getAttribute("aria-selected"),
  ).toBe("true");
  expect(change).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "应用时间范围" }));
  expect(change).toHaveBeenCalledWith("2026-10-08", "2026-10-12");
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "时间" })).toBe(
      document.activeElement,
    ),
  );
});

it("取消与Escape丢弃日期草稿，重新打开仍是已应用范围", async () => {
  const change = setup("2026-10-02", "2026-10-06");
  custom();
  fireEvent.click(day("2026年10月20日"));
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(change).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "时间" }));
  custom();
  expect(screen.getByRole("button", { name: "开始日期" }).textContent).toBe(
    "2026-10-02",
  );
  fireEvent.keyDown(day("2026年10月2日"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("grid")).toBeNull());
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "时间" })).toBe(
      document.activeElement,
    ),
  );
});

it("倒序和缺端显示错误，点击字段可修正结束日期", () => {
  const change = setup();
  custom();
  fireEvent.click(day("2026年10月20日"));
  fireEvent.click(screen.getByRole("button", { name: "应用时间范围" }));
  expect(screen.getByRole("alert").textContent).toContain("开始日期和结束日期");
  fireEvent.click(day("2026年10月10日"));
  expect(screen.queryByText(/可应用时间范围/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "应用时间范围" }));
  expect(screen.getByRole("alert").textContent).toContain(
    "开始日期不能晚于结束日期",
  );
  expect(change).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "结束日期" }));
  fireEvent.click(day("2026年10月28日"));
  fireEvent.click(screen.getByRole("button", { name: "应用时间范围" }));
  expect(change).toHaveBeenCalledWith("2026-10-20", "2026-10-28");
});

it("跨月保留草稿，支持任意前后月和键盘roving焦点", async () => {
  setup("2024-01-31", "2024-03-02");
  custom();
  const start = day("2024年1月31日");
  await waitFor(() => expect(start).toBe(document.activeElement));
  fireEvent.keyDown(start, { key: "PageDown" });
  await waitFor(() =>
    expect(day("2024年2月29日")).toBe(document.activeElement),
  );
  fireEvent.keyDown(day("2024年2月29日"), { key: "ArrowRight" });
  await waitFor(() => expect(day("2024年3月1日")).toBe(document.activeElement));
  fireEvent.keyDown(day("2024年3月1日"), { key: "Home" });
  await waitFor(() =>
    expect(day("2024年2月25日")).toBe(document.activeElement),
  );
  fireEvent.keyDown(day("2024年2月25日"), { key: "End" });
  await waitFor(() => expect(day("2024年3月2日")).toBe(document.activeElement));
  expect(
    screen.getByRole("grid").querySelectorAll('button[tabindex="0"]'),
  ).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "下一月" }));
  expect(screen.getByRole("grid").getAttribute("aria-label")).toBe("2024年4月");
  fireEvent.click(screen.getByRole("button", { name: "上一月" }));
  expect(screen.getByRole("button", { name: "结束日期" }).textContent).toBe(
    "2024-03-02",
  );
});

it("快捷范围按真实本地日期应用，全部清除已应用区间", () => {
  const change = setup("2026-10-01", "2026-10-20");
  fireEvent.click(screen.getByRole("button", { name: "最近一周" }));
  expect(change).toHaveBeenCalledWith("2026-10-22", "2026-10-28");
  fireEvent.click(screen.getByRole("button", { name: "时间" }));
  fireEvent.click(screen.getByRole("button", { name: "全部" }));
  expect(change).toHaveBeenLastCalledWith("", "");
});

it("最近一个月在月末回退至上月最后一天", () => {
  vi.setSystemTime(new Date(2024, 2, 31, 12));
  const change = setup();
  fireEvent.click(screen.getByRole("button", { name: "最近一个月" }));
  expect(change).toHaveBeenCalledWith("2024-02-29", "2024-03-31");
});

it("跨年月份切换不回到同年，并允许同一天的闭合区间", () => {
  const change = setup("2026-12-31", "2026-12-31");
  custom();
  fireEvent.click(screen.getByRole("button", { name: "下一月" }));
  expect(screen.getByRole("grid").getAttribute("aria-label")).toBe("2027年1月");
  fireEvent.click(screen.getByRole("button", { name: "上一月" }));
  expect(screen.getByRole("grid").getAttribute("aria-label")).toBe(
    "2026年12月",
  );
  fireEvent.click(screen.getByRole("button", { name: "应用时间范围" }));
  expect(change).toHaveBeenCalledWith("2026-12-31", "2026-12-31");
});

it("工具栏显示已应用范围，清除只更改日期且保留收藏和排序", () => {
  const change = vi.fn();
  render(
    <UnifiedSubjectToolbar
      value={{
        favorite: true,
        order: "created_asc",
        from: "2026-10-01",
        through: "2026-11-05",
      }}
      disabled={false}
      onChange={change}
      onBatch={vi.fn()}
    />,
  );
  expect(screen.getByText("2026-10-01")).toBeTruthy();
  expect(screen.getByText("2026-11-05")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "清除时间" }));
  expect(change).toHaveBeenCalledWith({
    favorite: true,
    order: "created_asc",
    from: "",
    through: "",
  });
});
