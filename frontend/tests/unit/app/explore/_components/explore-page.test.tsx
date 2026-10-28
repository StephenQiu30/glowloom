import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { ExplorePage } from "@/app/explore/_components/explore-page";
import { PageToolbarContext } from "@/components/layout/page-toolbar";

afterEach(cleanup);

function selectCategory(name: string) {
  fireEvent.mouseDown(screen.getByRole("tab", { name }), {
    button: 0,
    ctrlKey: false,
  });
}

it("places all categories in the shared header slot and defaults to short drama", () => {
  const header = document.createElement("header");
  document.body.append(header);
  const view = render(
    <PageToolbarContext.Provider value={header}>
      <ExplorePage />
    </PageToolbarContext.Provider>,
  );
  expect(
    Array.from(header.querySelectorAll('[role="tab"]')).map(
      (tab) => tab.textContent,
    ),
  ).toEqual([
    "短剧漫剧",
    "影视作品",
    "动画动漫",
    "广告营销",
    "平面设计",
    "电商",
    "其他",
    "技能",
    "活动",
  ]);
  expect(
    screen.getByRole("tab", { name: "短剧漫剧" }).getAttribute("aria-selected"),
  ).toBe("true");
  expect(
    screen
      .getByRole("radio", { name: "短剧作品" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(screen.getByText("作品尚未开放")).toBeTruthy();
  view.unmount();
  expect(header.childElementCount).toBe(0);
  header.remove();
});

it("retains the current scene on reselection but resets after a category change", () => {
  render(<ExplorePage />);
  selectCategory("影视作品");
  fireEvent.click(screen.getByRole("radio", { name: "视觉设定" }));
  selectCategory("影视作品");
  expect(
    screen
      .getByRole("radio", { name: "视觉设定" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  selectCategory("短剧漫剧");
  expect(
    screen
      .getByRole("radio", { name: "短剧作品" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  selectCategory("影视作品");
  expect(
    screen
      .getByRole("radio", { name: "短片作品" })
      .getAttribute("aria-checked"),
  ).toBe("true");
});

it("separates skill categories and removes work controls from activities", () => {
  render(<ExplorePage />);
  selectCategory("技能");
  expect(screen.getAllByRole("radio")).toHaveLength(12);
  expect(
    screen
      .getByRole("radio", { name: "热门推荐" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(screen.getByText("技能尚未开放")).toBeTruthy();
  selectCategory("活动");
  expect(screen.queryByRole("radiogroup")).toBeNull();
  expect(screen.queryByRole("button", { name: "发布短片" })).toBeNull();
  expect(screen.getByText("活动尚未开放")).toBeTruthy();
});

it("filters labeled examples by the exact advertising scene", () => {
  render(<ExplorePage />);
  selectCategory("广告营销");
  expect(screen.getAllByText("AI 生成示例")).toHaveLength(4);
  expect(
    screen.queryByRole("button", { name: /声色之间 · 耳机广告/ }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("radio", { name: "社媒营销" }));
  expect(screen.getAllByText("AI 生成示例")).toHaveLength(4);
  expect(
    screen.getByRole("button", { name: /声色之间 · 耳机广告/ }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: /声色之间 · 产品概念/ }),
  ).toBeNull();
});
