import { useState } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ExploreSearch } from "@/components/layout/explore-search";
import { SessionContext } from "@/components/auth/session";
afterEach(() => {
  cleanup();
  localStorage.clear();
});
function Fixture({ submit = vi.fn() }: { submit?: (value: string) => void }) {
  const [value, onValueChange] = useState("");
  return (
    <ExploreSearch
      search={{ value, onValueChange, onSearch: submit, label: "搜索" }}
    />
  );
}
it("opens inline, waits for explicit submit and ignores blank or composing Enter", () => {
  const submit = vi.fn();
  render(<Fixture submit={submit} />);
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  const input = screen.getByRole("textbox", { name: "搜索" });
  fireEvent.change(input, { target: { value: "   " } });
  fireEvent.submit(screen.getByRole("search"));
  expect(submit).not.toHaveBeenCalled();
  fireEvent.change(input, { target: { value: " 海报 " } });
  fireEvent.keyDown(input, { key: "Enter", isComposing: true, keyCode: 229 });
  expect(submit).not.toHaveBeenCalled();
  fireEvent.submit(screen.getByRole("search"));
  expect(submit).toHaveBeenCalledWith("海报");
  expect(screen.queryByRole("dialog")).toBeNull();
});
it("restores history, reuses keywords and persists clearing", async () => {
  localStorage.setItem(
    "glowloom:explore-search:guest",
    JSON.stringify(["海报", "风景"]),
  );
  const submit = vi.fn();
  render(<Fixture submit={submit} />);
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "海报" })).toBeTruthy(),
  );
  fireEvent.click(screen.getByRole("button", { name: "海报" }));
  expect(submit).toHaveBeenCalledWith("海报");
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "" } });
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "清空" })).toBeTruthy(),
  );
  fireEvent.click(screen.getByRole("button", { name: "清空" }));
  expect(localStorage.getItem("glowloom:explore-search:guest")).toBe("[]");
});
it("deduplicates recent history and tolerates corrupted storage", () => {
  localStorage.setItem("glowloom:explore-search:guest", "broken");
  const submit = vi.fn();
  render(<Fixture submit={submit} />);
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  const input = screen.getByRole("textbox");
  fireEvent.change(input, { target: { value: "海报" } });
  fireEvent.submit(screen.getByRole("search"));
  fireEvent.submit(screen.getByRole("search"));
  expect(
    JSON.parse(localStorage.getItem("glowloom:explore-search:guest")!),
  ).toEqual(["海报"]);
});
it("does not reveal guest history to a signed-in account", () => {
  localStorage.setItem(
    "glowloom:explore-search:guest",
    JSON.stringify(["游客关键词"]),
  );
  const session = { actor: { id: "actor-1" } } as API.EmailSessionView;
  render(
    <SessionContext.Provider value={session}>
      <Fixture />
    </SessionContext.Provider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  expect(screen.queryByText("游客关键词")).toBeNull();
});
