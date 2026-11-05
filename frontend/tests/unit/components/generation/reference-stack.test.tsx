import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useState } from "react";
import { ReferenceStack } from "@/components/generation/reference-stack";
afterEach(cleanup);
it("adds from the empty card without changing a draft itself", () => {
  const add = vi.fn();
  render(
    <ReferenceStack
      entries={[]}
      onAdd={add}
      onSelect={() => {}}
      onRemove={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "添加参考" }));
  expect(add).toHaveBeenCalledTimes(1);
});
it("removes the selected identity and leaves the other reference intact", () => {
  function Host() {
    const [entries, setEntries] = useState([
      { id: "product", label: "产品图", preview: <span>产品</span> },
      { id: "whale", label: "风格图", preview: <span>鲸鱼</span> },
    ]);
    return (
      <ReferenceStack
        entries={entries}
        onAdd={() => {}}
        onSelect={() => {}}
        onRemove={(id) =>
          setEntries((items) => items.filter((item) => item.id !== id))
        }
      />
    );
  }
  render(<Host />);
  fireEvent.click(screen.getByRole("button", { name: "展开参考，已添加2项" }));
  fireEvent.click(screen.getByRole("button", { name: "移除产品图" }));
  expect(screen.queryByRole("button", { name: "移除产品图" })).toBeNull();
  expect(screen.getByRole("button", { name: "移除风格图" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "移除风格图" }));
  expect(screen.getByRole("button", { name: "添加参考" })).toBeTruthy();
});
it("has touch and keyboard equivalents for hover expansion", () => {
  const select = vi.fn();
  render(
    <ReferenceStack
      entries={[{ id: "one", label: "图片1", preview: <span>image</span> }]}
      onAdd={() => {}}
      onSelect={select}
      onRemove={() => {}}
    />,
  );
  const main = screen.getByRole("button", { name: "展开参考，已添加1项" });
  fireEvent.click(main);
  fireEvent.click(screen.getByRole("button", { name: "查看图片1" }));
  expect(select).toHaveBeenCalledWith("one");
  fireEvent.keyDown(main, { key: "Escape" });
  expect(screen.queryByRole("button", { name: "查看图片1" })).toBeNull();
});
