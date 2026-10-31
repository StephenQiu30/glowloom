import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasTextActions } from "@/components/canvas/nodes/canvas-text-actions";
import type { ComponentProps } from "react";

vi.mock("@/components/canvas/hooks/use-bound-references", () => ({
  useBoundReferences: () => [],
}));
afterEach(cleanup);
function props(): ComponentProps<typeof CanvasTextActions> {
  return {
    node: {
      id: "text-1",
      type: "text",
      title: "文本 1",
      position: { x: 300, y: 200 },
      width: 320,
      height: 320,
      metadata: { content: "正文", textBackground: "cyan" },
    },
    viewport: { x: 0, y: 0, k: 1.36 },
    canvasSize: { width: 1440, height: 900 },
    onBackgroundChange: vi.fn(),
    onContentChange: vi.fn(),
    acceptContent: () => true,
    onKeep: vi.fn(),
    onLeave: vi.fn(),
  };
}

it("shows the saved color, commits a palette choice, and restores trigger focus", async () => {
  const p = props();
  render(<CanvasTextActions {...p} />);
  fireEvent.keyDown(screen.getByRole("button", { name: "背景色" }), {
    key: "Enter",
  });
  expect(screen.getAllByRole("menuitemradio")).toHaveLength(6);
  expect(
    screen
      .getByRole("menuitemradio", { name: "青绿色" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("menuitemradio", { name: "无颜色" }));
  expect(p.onBackgroundChange).toHaveBeenCalledWith("default");
  expect(screen.queryByRole("menu")).toBeNull();
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "背景色" }),
    ),
  );
});

it("does not write after permission becomes read-only while the palette is open", async () => {
  const p = props();
  const view = render(<CanvasTextActions {...p} />);
  fireEvent.keyDown(screen.getByRole("button", { name: "背景色" }), {
    key: "Enter",
  });
  view.rerender(<CanvasTextActions {...p} readOnly />);
  fireEvent.click(screen.getByRole("menuitemradio", { name: "紫色" }));
  expect(p.onBackgroundChange).not.toHaveBeenCalled();
  expect(
    screen.getByRole("button", { name: "背景色" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen.getByRole("button", { name: "下载文本" }).hasAttribute("disabled"),
  ).toBe(false);
});
