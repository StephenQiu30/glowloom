import { useState } from "react";
import { render } from "../../../feedback";
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { ProjectPickerFilters } from "@/components/media/project-picker-filters";
import { defaultProjectPickerFilter } from "@/components/media/project-picker";
afterEach(cleanup);
function Fixture() {
  const [value, setValue] = useState(defaultProjectPickerFilter);
  return (
    <>
      <ProjectPickerFilters kind="image" value={value} onChange={setValue} />
      <output>{JSON.stringify(value)}</output>
    </>
  );
}
it("规格支持多选、取消、总计数和各组清空，互不清除其他组", () => {
  render(<Fixture />);
  fireEvent.click(screen.getByRole("button", { name: "来源与规格筛选" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "1K" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "2K" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "16:9" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "1:1" }));
  expect(
    screen.getByRole("button", { name: "来源与规格筛选，已选 4 项" }),
  ).toBeTruthy();
  expect(
    within(screen.getByRole("group", { name: "图片比例" })).getByText("已选 2"),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("checkbox", { name: "1K" }));
  expect(
    screen.getByRole("button", { name: "来源与规格筛选，已选 3 项" }),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "清空图片比例" }));
  expect(
    screen.getByRole("checkbox", { name: "2K" }).getAttribute("data-state"),
  ).toBe("checked");
  expect(
    screen.getByRole("checkbox", { name: "16:9" }).getAttribute("data-state"),
  ).toBe("unchecked");
  fireEvent.click(screen.getByRole("button", { name: "清除筛选" }));
  expect(screen.getByRole("button", { name: "来源与规格筛选" })).toBeTruthy();
  expect(
    screen.getByRole("checkbox", { name: "2K" }).getAttribute("data-state"),
  ).toBe("unchecked");
});
