// Shortcut arbitration adapted from BeefTV use-canvas-keyboard.ts (MIT).
import { useEffect } from "react";
type Commands = {
  save: () => void;
  undo: () => void;
  redo: () => void;
  remove: () => void;
  copy: () => void;
  paste: () => void;
  selectAll: () => void;
  fit: () => void;
  fitSelection: () => void;
  zoom: (direction: number) => void;
  group: () => void;
  ungroup: () => void;
  arrange: () => void;
  cancel: () => void;
  search: () => void;
  move: (x: number, y: number) => void;
  tool: (value: "select" | "move") => void;
  zoomReset?: () => void;
  toggleFocus?: () => void;
  showShortcuts?: () => void;
};
export function hasCanvasTextSelection(selection: Selection | null) {
  return Boolean(
    selection &&
    !selection.isCollapsed &&
    selection.rangeCount > 0 &&
    selection.toString(),
  );
}
export function useCanvasKeyboard(commands: Commands, enabled: boolean) {
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (!enabled || event.defaultPrevented || event.isComposing) return;
      const target = event.target instanceof Element ? event.target : null;
      if (
        target?.closest("[role=dialog],[role=menu],[data-slot=select-content]")
      )
        return;
      const editing = target?.closest(
        "input,textarea,select,[contenteditable]:not([contenteditable=false]),[role=textbox]",
      );
      const modifier = event.metaKey || event.ctrlKey,
        key = event.key.toLowerCase();
      if (
        !modifier &&
        ["arrowleft", "arrowright", "arrowup", "arrowdown"].includes(key) &&
        target?.closest("button,[role=toolbar],[data-slot=toggle-group]")
      )
        return;
      if (modifier && key === "s") {
        event.preventDefault();
        if (!event.repeat) commands.save();
        return;
      }
      if (editing) return;
      if (event.altKey && modifier) return;
      if (modifier && key === "1" && commands.zoomReset) {
        event.preventDefault();
        commands.zoomReset();
        return;
      }
      if (modifier && (key === "2" || key === "3")) {
        event.preventDefault();
        if (key === "2") commands.fit();
        else commands.fitSelection();
        return;
      }
      if (!modifier && !event.altKey && key === "?" && commands.showShortcuts) {
        event.preventDefault();
        if (!event.repeat) commands.showShortcuts();
        return;
      }
      if (
        !modifier &&
        !event.altKey &&
        !event.shiftKey &&
        key === "tab" &&
        target?.matches("[data-canvas-viewport]") &&
        commands.toggleFocus
      ) {
        event.preventDefault();
        if (!event.repeat) commands.toggleFocus();
        return;
      }
      if (
        !modifier &&
        !event.altKey &&
        !event.shiftKey &&
        (key === "v" || key === "h")
      ) {
        event.preventDefault();
        commands.tool(key === "h" ? "move" : "select");
      } else if (modifier && key === "z") {
        event.preventDefault();
        if (event.shiftKey) commands.redo();
        else commands.undo();
      } else if (modifier && key === "y") {
        event.preventDefault();
        commands.redo();
      } else if (modifier && key === "c") {
        if (hasCanvasTextSelection(window.getSelection())) return;
        event.preventDefault();
        commands.copy();
      } else if (modifier && key === "v") {
        event.preventDefault();
        commands.paste();
      } else if (modifier && key === "a") {
        event.preventDefault();
        commands.selectAll();
      } else if (modifier && key === "g") {
        event.preventDefault();
        if (event.shiftKey) commands.ungroup();
        else commands.group();
      } else if (modifier && key === "f") {
        event.preventDefault();
        if (event.shiftKey && commands.toggleFocus) commands.toggleFocus();
        else commands.search();
      } else if (modifier && ["+", "=", "-"].includes(key)) {
        event.preventDefault();
        commands.zoom(key === "-" ? -1 : 1);
      } else if (modifier && key === "0") {
        event.preventDefault();
        commands.fit();
      } else if (key === "delete" || key === "backspace") {
        event.preventDefault();
        commands.remove();
      } else if (key === "escape") {
        event.preventDefault();
        commands.cancel();
      } else if (event.altKey && event.shiftKey && key === "f") {
        event.preventDefault();
        commands.arrange();
      } else if (key === "f" && !modifier && !event.altKey) {
        event.preventDefault();
        commands.fitSelection();
      } else if (
        ["arrowleft", "arrowright", "arrowup", "arrowdown"].includes(key)
      ) {
        event.preventDefault();
        const distance = event.shiftKey ? 10 : 1;
        commands.move(
          key === "arrowleft" ? -distance : key === "arrowright" ? distance : 0,
          key === "arrowup" ? -distance : key === "arrowdown" ? distance : 0,
        );
      }
    };
    window.addEventListener("keydown", keydown);
    return () => window.removeEventListener("keydown", keydown);
  }, [commands, enabled]);
}
