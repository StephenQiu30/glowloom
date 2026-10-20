import { expect, it } from "vitest";
import { isCanvasShortcut } from "./shortcut";

function key(target: Element, init: KeyboardEventInit = {}) {
  const event = new KeyboardEvent("keydown", {
    key: "v",
    bubbles: true,
    ...init,
  });
  target.dispatchEvent(event);
  return event;
}

it("画布视口接受快捷键，输入、弹层、输入法和连发不接受", () => {
  document.body.innerHTML = `<div data-canvas-viewport><button>工具</button><div id="board"></div></div>`;
  const board = document.getElementById("board")!;
  expect(isCanvasShortcut(key(board))).toBe(true);
  expect(isCanvasShortcut(key(document.querySelector("button")!))).toBe(false);
  expect(isCanvasShortcut(key(board, { isComposing: true }))).toBe(false);
  expect(isCanvasShortcut(key(board, { repeat: true }))).toBe(false);
  expect(isCanvasShortcut(key(board, { altKey: true }))).toBe(false);
  expect(isCanvasShortcut(key(document.body))).toBe(false);
  const dialog = document.createElement("div");
  dialog.setAttribute("role", "dialog");
  document.body.append(dialog);
  expect(isCanvasShortcut(key(board))).toBe(false);
  dialog.setAttribute("data-state", "closed");
  expect(isCanvasShortcut(key(board))).toBe(true);
  document.body.replaceChildren();
});
