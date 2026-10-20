// Shortcut scope from the source canvas keyboard lib.
// The persistent composer stays usable; menus, dialogs and text fields do not take canvas keys.
const controls =
  "input,textarea,select,button,a,[contenteditable='true'],[role='textbox'],[data-canvas-shortcuts-ignore],[data-canvas-no-zoom],[data-canvas-ai-panel]";
const overlays =
  '[role="dialog"],[role="alertdialog"],[role="menu"],[role="listbox"],[data-slot="popover-content"]';

export function isImeComposing(event: KeyboardEvent) {
  return event.isComposing || event.keyCode === 229;
}

export function hasCanvasOverlay(root: Document) {
  return [...root.querySelectorAll(overlays)].some(
    (element) =>
      !element.classList.contains("canvas-node-composer") &&
      !element.closest('[hidden],[aria-hidden="true"],[data-state="closed"]'),
  );
}

export function isCanvasKeyboardTarget(target: EventTarget | null) {
  return (
    target instanceof Element &&
    !!target.closest("[data-canvas-viewport]") &&
    !target.closest(controls)
  );
}

export function isCanvasShortcut(event: KeyboardEvent) {
  const root =
    event.target instanceof Element ? event.target.ownerDocument : document;
  return (
    !event.defaultPrevented &&
    !isImeComposing(event) &&
    !event.repeat &&
    !event.altKey &&
    isCanvasKeyboardTarget(event.target) &&
    !hasCanvasOverlay(root)
  );
}
