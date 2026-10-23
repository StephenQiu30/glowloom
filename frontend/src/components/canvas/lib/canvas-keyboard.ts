import { isImeComposing } from "@/components/canvas/runtime/lib/keyboard-event";

const controls =
  "input,textarea,select,button,a,[contenteditable='true'],[role='textbox'],[data-canvas-shortcuts-ignore],[data-canvas-no-zoom]";
const overlays =
  '[role="dialog"],[role="alertdialog"],[role="menu"],[role="listbox"],[data-slot="popover-content"]';

/** The persistent node composer is canvas UI; its nested menus are separate layers. */
export function hasCanvasOverlay(document: Document) {
  return [...document.querySelectorAll(overlays)].some(
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
  return (
    !event.defaultPrevented &&
    !isImeComposing(event) &&
    !event.repeat &&
    isCanvasKeyboardTarget(event.target) &&
    !hasCanvasOverlay((event.target as Element).ownerDocument)
  );
}
