import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useCanvasKeyboard } from "./keyboard";
afterEach(() => {
  cleanup();
  window.getSelection()?.removeAllRanges();
  document.body.replaceChildren();
});
function actions() {
  return {
    save: vi.fn(),
    undo: vi.fn(),
    redo: vi.fn(),
    remove: vi.fn(),
    copy: vi.fn(),
    paste: vi.fn(),
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
    tool: vi.fn(),
    zoomReset: vi.fn(),
    toggleFocus: vi.fn(),
    showShortcuts: vi.fn(),
  };
}
it("浏览器文字选区保留原生复制，清空选区后才能复制画布节点", () => {
  const commands = actions();
  renderHook(() => useCanvasKeyboard(commands, true));
  const text = document.createElement("p");
  text.textContent = "需要原生复制的文字";
  document.body.append(text);
  const range = document.createRange();
  range.selectNodeContents(text);
  window.getSelection()!.addRange(range);
  const copyText = new KeyboardEvent("keydown", {
    key: "c",
    metaKey: true,
    cancelable: true,
  });
  window.dispatchEvent(copyText);
  expect(commands.copy).not.toHaveBeenCalled();
  expect(copyText.defaultPrevented).toBe(false);
  window.getSelection()!.removeAllRanges();
  window.dispatchEvent(
    new KeyboardEvent("keydown", { key: "c", ctrlKey: true, cancelable: true }),
  );
  expect(commands.copy).toHaveBeenCalledOnce();
  text.remove();
});
it("Cmd1/2/3提供100%/全画布/选择适应，问号打开帮助", () => {
  const commands = actions();
  renderHook(() => useCanvasKeyboard(commands, true));
  for (const key of ["1", "2", "3"]) {
    const event = new KeyboardEvent("keydown", {
      key,
      ctrlKey: true,
      cancelable: true,
    });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  }
  window.dispatchEvent(
    new KeyboardEvent("keydown", { key: "?", shiftKey: true }),
  );
  expect(commands.zoomReset).toHaveBeenCalledOnce();
  expect(commands.fit).toHaveBeenCalledOnce();
  expect(commands.fitSelection).toHaveBeenCalledOnce();
  expect(commands.showShortcuts).toHaveBeenCalledOnce();
});
it("Tab仅在直接画布焦点切换专注，按钮输入弹窗及ShiftTab保持原生导航", () => {
  const commands = actions();
  renderHook(() => useCanvasKeyboard(commands, true));
  const canvas = document.createElement("div");
  canvas.setAttribute("data-canvas-viewport", "");
  const button = document.createElement("button"),
    input = document.createElement("input"),
    dialog = document.createElement("div");
  dialog.setAttribute("role", "dialog");
  canvas.append(button, input);
  document.body.append(canvas, dialog);
  const tab = new KeyboardEvent("keydown", {
    key: "Tab",
    bubbles: true,
    cancelable: true,
  });
  canvas.dispatchEvent(tab);
  expect(tab.defaultPrevented).toBe(true);
  expect(commands.toggleFocus).toHaveBeenCalledOnce();
  for (const target of [button, input, dialog, document.body]) {
    const event = new KeyboardEvent("keydown", {
      key: "Tab",
      bubbles: true,
      cancelable: true,
    });
    target.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    if (target === input || target === dialog) {
      for (const key of ["1", "?"]) {
        target.dispatchEvent(
          new KeyboardEvent("keydown", {
            key,
            ctrlKey: key === "1",
            bubbles: true,
          }),
        );
      }
    }
  }
  const backwards = new KeyboardEvent("keydown", {
    key: "Tab",
    shiftKey: true,
    bubbles: true,
    cancelable: true,
  });
  canvas.dispatchEvent(backwards);
  expect(backwards.defaultPrevented).toBe(false);
  expect(commands.toggleFocus).toHaveBeenCalledOnce();
  expect(commands.zoomReset).not.toHaveBeenCalled();
  expect(commands.showShortcuts).not.toHaveBeenCalled();
  canvas.remove();
  dialog.remove();
});
it("快捷键只调用最新实例并在卸载、禁用和输入法组合期间退出", () => {
  const first = actions(),
    second = actions();
  const hook = renderHook(
    ({ commands, enabled }) => useCanvasKeyboard(commands, enabled),
    { initialProps: { commands: first, enabled: true } },
  );
  hook.rerender({ commands: second, enabled: true });
  window.dispatchEvent(
    new KeyboardEvent("keydown", { key: "1", metaKey: true }),
  );
  expect(first.zoomReset).not.toHaveBeenCalled();
  expect(second.zoomReset).toHaveBeenCalledOnce();
  window.dispatchEvent(
    new KeyboardEvent("keydown", { key: "?", isComposing: true }),
  );
  window.dispatchEvent(
    new KeyboardEvent("keydown", { key: "?", repeat: true }),
  );
  expect(second.showShortcuts).not.toHaveBeenCalled();
  hook.rerender({ commands: second, enabled: false });
  window.dispatchEvent(
    new KeyboardEvent("keydown", { key: "1", metaKey: true }),
  );
  expect(second.zoomReset).toHaveBeenCalledOnce();
  hook.unmount();
  window.dispatchEvent(new KeyboardEvent("keydown", { key: "?" }));
  expect(second.showShortcuts).not.toHaveBeenCalled();
});
it("工具按钮方向键不移动节点，画布方向键和全局保存仍有效", () => {
  const commands = {
    save: vi.fn(),
    undo: vi.fn(),
    redo: vi.fn(),
    remove: vi.fn(),
    copy: vi.fn(),
    paste: vi.fn(),
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
    tool: vi.fn(),
  };
  renderHook(() => useCanvasKeyboard(commands, true));
  const toolbar = document.createElement("div");
  toolbar.setAttribute("role", "toolbar");
  const button = document.createElement("button");
  toolbar.append(button);
  document.body.append(toolbar);
  try {
    button.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "ArrowRight",
        bubbles: true,
        cancelable: true,
      }),
    );
    expect(commands.move).not.toHaveBeenCalled();
    button.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "s",
        ctrlKey: true,
        bubbles: true,
        cancelable: true,
      }),
    );
    expect(commands.save).toHaveBeenCalledOnce();
    window.dispatchEvent(
      new KeyboardEvent("keydown", { key: "ArrowRight", cancelable: true }),
    );
    expect(commands.move).toHaveBeenCalledExactlyOnceWith(1, 0);
    const handled = new KeyboardEvent("keydown", {
      key: "ArrowLeft",
      cancelable: true,
    });
    handled.preventDefault();
    window.dispatchEvent(handled);
    expect(commands.move).toHaveBeenCalledOnce();
  } finally {
    toolbar.remove();
  }
});
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
