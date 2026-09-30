import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useCanvasKeyboard } from "./keyboard";
afterEach(cleanup);
it("V/H切换原生画布模式，不抢占文字输入、粘贴或对话框", () => {
  const tool = vi.fn(),
    paste = vi.fn();
  const commands = {
    save: vi.fn(),
    undo: vi.fn(),
    redo: vi.fn(),
    remove: vi.fn(),
    copy: vi.fn(),
    paste,
    selectAll: vi.fn(),
    fit: vi.fn(),
    fitSelection: vi.fn(),
    zoom: vi.fn(),
    group: vi.fn(),
    ungroup: vi.fn(),
    arrange: vi.fn(),
    cancel: vi.fn(),
    search: vi.fn(),
    move: vi.fn(),
    tool,
  };
  renderHook(() => useCanvasKeyboard(commands, true));
  window.dispatchEvent(new KeyboardEvent("keydown", { key: "h" }));
  window.dispatchEvent(new KeyboardEvent("keydown", { key: "v" }));
  expect(tool.mock.calls).toEqual([["move"], ["select"]]);
  window.dispatchEvent(
    new KeyboardEvent("keydown", { key: "v", ctrlKey: true }),
  );
  expect(paste).toHaveBeenCalledOnce();
  const input = document.createElement("textarea"),
    dialog = document.createElement("div");
  dialog.setAttribute("role", "dialog");
  dialog.append(input);
  document.body.append(dialog);
  input.dispatchEvent(
    new KeyboardEvent("keydown", { key: "h", bubbles: true }),
  );
  dialog.dispatchEvent(
    new KeyboardEvent("keydown", { key: "v", bubbles: true }),
  );
  expect(tool).toHaveBeenCalledTimes(2);
  dialog.remove();
});
