import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ExplorePage } from "@/app/explore/_components/explore-page";
import { PageToolbarContext } from "@/components/layout/page-toolbar";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
afterEach(cleanup);

function selectTab(name: string) {
  fireEvent.mouseDown(screen.getByRole("tab", { name }), {
    button: 0,
    ctrlKey: false,
  });
}

it("places all categories in the shared header slot and starts with featured work", () => {
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
    "精选",
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
    screen.getByRole("tab", { name: "精选" }).getAttribute("aria-selected"),
  ).toBe("true");
  expect(screen.queryByRole("tablist", { name: "作品场景" })).toBeNull();
  expect(
    screen.getByRole("button", { name: "声色之间的产品灵感" }),
  ).toBeTruthy();
  view.unmount();
  expect(header.childElementCount).toBe(0);
  header.remove();
});

it("retains the current scene on reselection but resets after a category change", () => {
  render(<ExplorePage />);
  selectTab("影视作品");
  selectTab("视觉设定");
  selectTab("影视作品");
  expect(
    screen.getByRole("tab", { name: "视觉设定" }).getAttribute("aria-selected"),
  ).toBe("true");
  selectTab("短剧漫剧");
  expect(
    screen.getByRole("tab", { name: "短剧作品" }).getAttribute("aria-selected"),
  ).toBe("true");
  selectTab("影视作品");
  expect(
    screen.getByRole("tab", { name: "短片作品" }).getAttribute("aria-selected"),
  ).toBe("true");
});

it("separates skill categories and removes work controls from activities", () => {
  render(<ExplorePage />);
  selectTab("技能");
  expect(
    within(screen.getByRole("tablist", { name: "技能分类" })).getAllByRole(
      "tab",
    ),
  ).toHaveLength(12);
  expect(
    screen.getByRole("tab", { name: "热门推荐" }).getAttribute("aria-selected"),
  ).toBe("true");
  expect(screen.getByText("技能尚未开放")).toBeTruthy();
  selectTab("活动");
  expect(screen.queryByRole("tablist", { name: "作品场景" })).toBeNull();
  expect(screen.queryByRole("button", { name: "发布短片" })).toBeNull();
  expect(screen.queryByRole("tablist", { name: "技能分类" })).toBeNull();
  expect(screen.getByText("活动尚未开放")).toBeTruthy();
});

it("filters labeled examples by the exact advertising scene", () => {
  render(<ExplorePage />);
  selectTab("广告营销");
  expect(screen.getAllByText("AI 生成示例")).toHaveLength(1);
  expect(
    screen.queryByRole("button", { name: /声色之间 · 耳机广告/ }),
  ).toBeNull();
  selectTab("社媒营销");
  expect(screen.getAllByText("AI 生成示例")).toHaveLength(1);
  expect(
    screen.getByRole("button", { name: /声色之间 · 耳机广告/ }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: /声色之间 · 产品概念/ }),
  ).toBeNull();
});
