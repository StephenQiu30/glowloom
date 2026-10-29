import { useState } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SubjectDateRange } from "@/components/subjects/subject-date-range";

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(2026, 9, 28, 12));
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
function setup(from = "", through = "") {
  const changed = vi.fn();
  function Fixture() {
    const [range, setRange] = useState({ from, through });
    return (
      <SubjectDateRange
        {...range}
        disabled={false}
        onChange={(first, last) => {
          changed(first, last);
          setRange({ from: first, through: last });
        }}
      />
    );
  }
  render(<Fixture />);
  fireEvent.click(screen.getByRole("button", { name: "时间" }));
  return changed;
}
const day = (value: string) => screen.getByRole("button", { name: value });
const openStart = () =>
  fireEvent.click(screen.getByRole("button", { name: "开始日期" }));
const openEnd = () =>
  fireEvent.click(screen.getByRole("button", { name: "结束日期" }));

it("时间菜单显示两个独立日期入口，结束日期默认今天，不增加应用或取消步骤", () => {
  setup();
  expect(screen.getByRole("button", { name: "开始日期" }).textContent).toBe(
    "开始日期",
  );
  expect(screen.getByRole("button", { name: "结束日期" }).textContent).toBe(
    "26-10-28",
  );
  expect(screen.queryByRole("button", { name: "应用时间范围" })).toBeNull();
  expect(screen.queryByRole("button", { name: "取消" })).toBeNull();
});

it("选择开始日期立即应用至今天，关闭小日历而保留时间菜单", async () => {
  const changed = setup();
  openStart();
  fireEvent.click(day("2026年10月8日"));
  expect(changed).toHaveBeenLastCalledWith("2026-10-08", "2026-10-28");
  await waitFor(() => expect(screen.queryByRole("grid")).toBeNull());
  expect(screen.getByRole("button", { name: "时间" }).textContent).toContain(
    "26-10-08 至 26-10-28",
  );
  expect(screen.getByRole("button", { name: "全部" })).toBeTruthy();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "开始日期" })).toBe(
      document.activeElement,
    ),
  );
});

it("选择结束日期立即生效，禁止倒序，允许同日范围", () => {
  const changed = setup("2026-10-20", "2026-10-28");
  openEnd();
  expect((day("2026年10月19日") as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(day("2026年10月19日"));
  expect(changed).not.toHaveBeenCalled();
  fireEvent.click(day("2026年10月20日"));
  expect(changed).toHaveBeenLastCalledWith("2026-10-20", "2026-10-20");
});

it("Escape仅关闭当前日历，不修改已应用范围，也没有额外未应用草稿", async () => {
  const changed = setup("2026-10-02", "2026-10-06");
  openStart();
  fireEvent.keyDown(day("2026年10月2日"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("grid")).toBeNull());
  expect(changed).not.toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "全部" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "开始日期" }).textContent).toBe(
    "26-10-02",
  );
});

it("快捷范围实时生效并保持展开，标记当前选项；全部清除区间", () => {
  const changed = setup();
  fireEvent.click(screen.getByRole("button", { name: "最近一周" }));
  expect(changed).toHaveBeenLastCalledWith("2026-10-22", "2026-10-28");
  expect(screen.getByRole("button", { name: "时间" }).textContent).toContain(
    "最近一周",
  );
  expect(
    screen
      .getByRole("button", { name: "最近一周" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "全部" }));
  expect(changed).toHaveBeenLastCalledWith("", "");
  expect(screen.getByRole("button", { name: "时间" }).textContent).toContain(
    "时间",
  );
  expect(
    screen.getByRole("button", { name: "全部" }).getAttribute("aria-pressed"),
  ).toBe("true");
});

it("顶栏原位清空不需要正文条件条", () => {
  const changed = setup("2026-10-01", "2026-10-20");
  fireEvent.click(screen.getByRole("button", { name: "清空时间" }));
  expect(changed).toHaveBeenLastCalledWith("", "");
  expect(screen.queryByRole("button", { name: "清空时间" })).toBeNull();
});

it("最近一个月在月末回退至上月最后一天", () => {
  vi.setSystemTime(new Date(2024, 2, 31, 12));
  const changed = setup();
  fireEvent.click(screen.getByRole("button", { name: "最近一个月" }));
  expect(changed).toHaveBeenLastCalledWith("2024-02-29", "2024-03-31");
});

it("日历支持跨月闰年、键盘roving焦点及相邻年月导航", async () => {
  setup("2024-01-31", "2024-03-02");
  openStart();
  await waitFor(() =>
    expect(day("2024年1月31日")).toBe(document.activeElement),
  );
  fireEvent.keyDown(day("2024年1月31日"), { key: "PageDown" });
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
    "24-03-02",
  );
});

it("预设不写回自定义日期入口，空自定义仍显示开始占位与今天", () => {
  const changed = setup();
  fireEvent.click(screen.getByRole("button", { name: "最近一周" }));
  expect(screen.getByRole("button", { name: "开始日期" }).textContent).toBe(
    "开始日期",
  );
  expect(screen.getByRole("button", { name: "结束日期" }).textContent).toBe(
    "26-10-28",
  );
  openStart();
  fireEvent.click(day("2026年10月14日"));
  expect(changed).toHaveBeenLastCalledWith("2026-10-14", "2026-10-28");
  expect(screen.getByRole("button", { name: "时间" }).textContent).toContain(
    "26-10-14 至 26-10-28",
  );
});

it("预设保留此前自定义端点，继续选择自定义日期不会混入预设范围", () => {
  const changed = setup("2026-10-02", "2026-10-06");
  fireEvent.click(screen.getByRole("button", { name: "最近一周" }));
  expect(screen.getByRole("button", { name: "开始日期" }).textContent).toBe(
    "26-10-02",
  );
  expect(screen.getByRole("button", { name: "结束日期" }).textContent).toBe(
    "26-10-06",
  );
  openEnd();
  fireEvent.click(day("2026年10月10日"));
  expect(changed).toHaveBeenLastCalledWith("2026-10-02", "2026-10-10");
});
