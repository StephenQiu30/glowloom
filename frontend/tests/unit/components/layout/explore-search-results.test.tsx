import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ExploreSearchResults } from "@/app/explore/search/search-results";
const { push } = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
afterEach(() => {
  cleanup();
  push.mockReset();
});
it("restores the query and type, switches URL type without losing the submitted keyword", () => {
  render(<ExploreSearchResults query="海报" type="shortFilm" />);
  expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe("海报");
  expect(
    screen.getByRole("tab", { name: "短片" }).getAttribute("aria-selected"),
  ).toBe("true");
  fireEvent.mouseDown(screen.getByRole("tab", { name: "技能" }), {
    button: 0,
    ctrlKey: false,
  });
  expect(push).toHaveBeenCalledWith(
    "/explore/search?result=%E6%B5%B7%E6%8A%A5&type=skill",
  );
  expect(screen.getByText("短片搜索尚未开放")).toBeTruthy();
});
it("shows a start state for a direct empty query and submits an encoded keyword", () => {
  render(<ExploreSearchResults query="" type="inspiration" />);
  expect(screen.getByText("输入关键词开始搜索")).toBeTruthy();
  fireEvent.change(screen.getByRole("textbox"), {
    target: { value: "光 & 影" },
  });
  fireEvent.submit(screen.getByRole("search"));
  expect(push).toHaveBeenCalledWith(
    "/explore/search?result=%E5%85%89%20%26%20%E5%BD%B1&type=inspiration",
  );
});
