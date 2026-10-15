import {
  cleanup,
  fireEvent,
  render,
  screen,
  act,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";
import { useReadBoundary } from "./use-read-boundary";
afterEach(cleanup);
it("筛选变化后旧详情响应不能覆盖新选择", async () => {
  let resolve!: (v: string) => void;
  const slow = new Promise<string>((r) => (resolve = r));
  function View({ selection }: { selection: string }) {
    const [value, setValue] = useState("");
    const read = useReadBoundary(selection);
    return (
      <>
        <button onClick={() => void read(() => slow, setValue)}>读取</button>
        <span>{value}</span>
      </>
    );
  }
  const { rerender } = render(<View selection="old" />);
  fireEvent.click(screen.getByText("读取"));
  rerender(<View selection="new" />);
  await act(async () => resolve("old private detail"));
  expect(screen.queryByText("old private detail")).toBeNull();
});
